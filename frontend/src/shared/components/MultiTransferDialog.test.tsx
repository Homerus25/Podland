import { Text } from "@chakra-ui/react";
import {
	MultiTransferDialog,
	type MultiTransferItem,
	type MultiTransferOutcome,
	type MultiTransferRunContext,
} from "@shared/components/MultiTransferDialog";
import {
	type TransferPreviewSlice,
	type TransferState,
	useTransferState,
} from "@shared/components/TransferDialog";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";
import { hostFixture } from "@/test/factories";

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

// The batch dialog's host facts arrive through the same state hook the
// single-item preset composes; the stub stands in for the query so this
// suite pins the dialog, not the transport.
vi.mock("@features/hosts/hooks/useHosts", () => ({
	useHosts: () => ({ data: [source, staging, readOnly, backup] }),
}));

type Gadget = { key: string };

function stubPreview(
	overrides: Partial<TransferPreviewSlice> = {},
): TransferPreviewSlice {
	return {
		isPending: false,
		isError: false,
		error: null,
		data: { collision: false },
		...overrides,
	};
}

function stubTransfer(overrides: Partial<TransferState> = {}): TransferState {
	return {
		targets: [staging, backup],
		targetId: "m-2",
		targetName: "staging-01",
		selectTarget: vi.fn(),
		mode: "move",
		setMode: vi.fn(),
		...overrides,
	};
}

function gadgetItem(
	key: string,
	overrides: Partial<MultiTransferItem<Gadget>> = {},
): MultiTransferItem<Gadget> {
	return {
		item: { key },
		key,
		label: key,
		preview: stubPreview(),
		...overrides,
	};
}

// The dialog as an adapter rides it: the real state hook over the stubbed
// hosts, so target and mode interactions run through real state.
function MultiHarness(
	props: Omit<
		Parameters<typeof MultiTransferDialog<Gadget>>[0],
		"host" | "transfer"
	>,
) {
	const transfer = useTransferState(source, "manage-volumes");
	return <MultiTransferDialog host={source} transfer={transfer} {...props} />;
}

function renderDialog(
	overrides: Partial<Parameters<typeof MultiTransferDialog<Gadget>>[0]> = {},
) {
	const props = {
		noun: "gadget",
		items: [gadgetItem("g-1"), gadgetItem("g-2")],
		runItem: vi.fn(
			async (): Promise<MultiTransferOutcome> => ({
				ok: true,
				detail: "Moved",
			}),
		),
		onFinished: vi.fn(),
		onClose: vi.fn(),
		...overrides,
	};
	const view = render(<MultiHarness {...props} />);
	return { ...props, unmount: view.unmount };
}

// The batch contract (spec 486, ticket #487), pinned once here: one target
// and one mode for the whole selection, every item's preview and warnings
// under its own name, nothing running until the whole batch is confirmed,
// sequential client-side execution in list order over the per-item
// endpoints, a per-item report, and the succeeded keys handed back so the
// list can drop only those from its selection.
describe("MultiTransferDialog", () => {
	it("chooses the target and the mode once for the whole batch", async () => {
		const user = userEvent.setup();
		const contexts: MultiTransferRunContext[] = [];
		renderDialog({
			runItem: async (_item, ctx) => {
				contexts.push(ctx);
				return { ok: true, detail: "Moved" };
			},
		});

		// One select, one radio group: the ritual is the single dialog's,
		// chosen once for every item.
		expect(screen.getAllByTestId("move-target-select")).toHaveLength(1);
		expect(screen.getAllByTestId("move-mode-radio")).toHaveLength(1);
		const names = within(screen.getByTestId("move-target-select"))
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01", "backup-01"]);

		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's gadget)" }),
		);
		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		await waitFor(() => expect(contexts).toHaveLength(2));
		for (const context of contexts) {
			expect(context.targetId).toBe("m-4");
			expect(context.targetName).toBe("backup-01");
			expect(context.mode).toBe("copy");
		}
	});

	it("hides the mode radio and pins the batch to Copy when the mode is pinned", () => {
		renderDialog({ pinnedMode: "copy" });

		expect(screen.queryByTestId("move-mode-radio")).not.toBeInTheDocument();
		// The confirm label is count-free (spec 486's out-of-scope rule): the
		// noun and the count stay in the dialog title, never on a button.
		expect(
			screen.getByTestId("confirm-multi-transfer-button"),
		).toHaveTextContent(/^Copy$/);
	});

	it("renders every item's label, preview state, and warnings under its name", () => {
		renderDialog({
			items: [
				gadgetItem("g-1", {
					preview: stubPreview({ isPending: true }),
					warnings: <Text data-testid="g-1-warning">g-1 warning</Text>,
				}),
				gadgetItem("g-2", {
					preview: stubPreview({
						isError: true,
						error: axiosError(403, "ACCESS_DENIED", "Access denied", "req-71"),
					}),
				}),
				gadgetItem("g-3", {
					preview: stubPreview({ data: { collision: true } }),
				}),
			],
		});

		const blocks = screen.getAllByTestId("multi-transfer-item");
		expect(blocks).toHaveLength(3);

		expect(within(blocks[0]).getByText("g-1")).toBeInTheDocument();
		expect(
			within(blocks[0]).getByTestId("multi-transfer-item-pending"),
		).toBeInTheDocument();
		// The item's domain warnings render under its own name.
		expect(within(blocks[0]).getByTestId("g-1-warning")).toBeInTheDocument();

		expect(within(blocks[1]).getByText("g-2")).toBeInTheDocument();
		expect(
			within(blocks[1]).getByTestId("multi-transfer-item-error"),
		).toHaveTextContent("ACCESS_DENIED");

		expect(within(blocks[2]).getByText("g-3")).toBeInTheDocument();
		const collision = within(blocks[2]).getByTestId(
			"multi-transfer-item-collision",
		);
		expect(collision).toHaveTextContent("A gadget named “g-3”");
		expect(collision).toHaveTextContent("already exists on staging-01");
	});

	it("runs nothing until the whole batch is confirmed", async () => {
		const user = userEvent.setup();
		const props = renderDialog();

		expect(props.runItem).not.toHaveBeenCalled();
		await user.selectOptions(screen.getByTestId("move-target-select"), "m-4");
		expect(props.runItem).not.toHaveBeenCalled();
		expect(props.onFinished).not.toHaveBeenCalled();
	});

	it("keeps confirm disabled until every preview settles", () => {
		const pending = renderDialog({
			items: [
				gadgetItem("g-1"),
				gadgetItem("g-2", { preview: stubPreview({ isPending: true }) }),
			],
		});
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
		pending.unmount();

		const failed = renderDialog({
			items: [
				gadgetItem("g-1"),
				gadgetItem("g-2", {
					preview: stubPreview({
						isError: true,
						error: axiosError(403, "ACCESS_DENIED", "Access denied"),
					}),
				}),
			],
		});
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
		failed.unmount();

		const colliding = renderDialog({
			items: [
				gadgetItem("g-1"),
				gadgetItem("g-2", {
					preview: stubPreview({ data: { collision: true } }),
				}),
			],
		});
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
		colliding.unmount();

		// An empty batch has nothing to run.
		const empty = renderDialog({ items: [] });
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
		empty.unmount();

		// The adapter's own refusal.
		const refused = renderDialog({ confirmDisabled: true });
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
		refused.unmount();

		// No writable target means nothing to confirm against.
		render(
			<MultiTransferDialog
				host={source}
				transfer={stubTransfer({ targets: [], targetId: "", targetName: "" })}
				noun="gadget"
				items={[gadgetItem("g-1")]}
				runItem={vi.fn()}
				onFinished={vi.fn()}
				onClose={vi.fn()}
			/>,
		);
		expect(screen.getByTestId("move-no-targets")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-multi-transfer-button")).toBeDisabled();
	});

	it("runs the items sequentially in list order and reports each one", async () => {
		const user = userEvent.setup();
		const calls: string[] = [];
		const props = renderDialog({
			items: [gadgetItem("g-1"), gadgetItem("g-2"), gadgetItem("g-3")],
			runItem: async (item) => {
				calls.push(`start:${item.key}`);
				// A real async gap: a parallel run would interleave the starts
				// before any end.
				await Promise.resolve();
				calls.push(`end:${item.key}`);
				return {
					ok: item.key !== "g-2",
					detail: item.key === "g-2" ? "boom" : "Moved",
				};
			},
		});

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		const report = await screen.findByTestId("multi-transfer-report");
		expect(calls).toEqual([
			"start:g-1",
			"end:g-1",
			"start:g-2",
			"end:g-2",
			"start:g-3",
			"end:g-3",
		]);

		const rows = within(report).getAllByTestId("multi-transfer-report-row");
		expect(rows).toHaveLength(3);
		expect(within(rows[0]).getByText("g-1")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Moved")).toBeInTheDocument();
		expect(rows[0]).toHaveAttribute("data-status", "ok");
		expect(within(rows[1]).getByText("g-2")).toBeInTheDocument();
		expect(within(rows[1]).getByText("boom")).toBeInTheDocument();
		expect(rows[1]).toHaveAttribute("data-status", "failed");
		expect(within(rows[2]).getByText("g-3")).toBeInTheDocument();

		// The form is gone: the report replaced the body.
		expect(screen.queryByTestId("move-target-select")).not.toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-multi-transfer-button"),
		).not.toBeInTheDocument();
		expect(props.onFinished).toHaveBeenCalledTimes(1);
	});

	it("shows each item's live progress on its own report row", async () => {
		const user = userEvent.setup();
		let release: (() => void) | null = null;
		renderDialog({
			items: [gadgetItem("g-1"), gadgetItem("g-2")],
			runItem: (item, ctx) => {
				// Only the first item is held open, so the mid-flight state is
				// observable; the second resolves at once and the run finishes
				// inside the same act scope.
				if (item.key !== "g-1") {
					return Promise.resolve({ ok: true, detail: "Moved" });
				}
				return new Promise<MultiTransferOutcome>((resolve) => {
					ctx.onProgress(`uploading ${item.key}…`);
					release = () => resolve({ ok: true, detail: "Moved" });
				});
			},
		});

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		const rows = await screen.findAllByTestId("multi-transfer-report-row");
		expect(within(rows[0]).getByText("uploading g-1…")).toBeInTheDocument();
		expect(rows[0]).toHaveAttribute("data-status", "running");
		// The second item has not started: the run is sequential.
		expect(rows[1]).toHaveAttribute("data-status", "pending");
		expect(within(rows[1]).queryByText(/uploading/)).not.toBeInTheDocument();

		await act(async () => {
			release?.();
		});
		await waitFor(() =>
			expect(
				within(screen.getAllByTestId("multi-transfer-report-row")[0]).getByText(
					"Moved",
				),
			).toBeInTheDocument(),
		);
	});

	it("hands onFinished only the succeeded keys, once, and leaves the report up", async () => {
		const user = userEvent.setup();
		const props = renderDialog({
			items: [gadgetItem("g-1"), gadgetItem("g-2"), gadgetItem("g-3")],
			runItem: async (item) => ({
				ok: item.key !== "g-2",
				detail: item.key === "g-2" ? "boom" : "Moved",
			}),
		});

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));
		await screen.findByTestId("multi-transfer-report");

		// Only the successes leave the selection: the failure stays selected
		// for the retry.
		expect(props.onFinished).toHaveBeenCalledTimes(1);
		expect(props.onFinished).toHaveBeenCalledWith(["g-1", "g-3"]);

		// The report stays up behind a Close button.
		await user.click(screen.getByTestId("close-multi-transfer-report-button"));
		expect(props.onClose).toHaveBeenCalledTimes(1);
	});

	it("aborts the run and closes when the dialog is closed mid-run", async () => {
		const user = userEvent.setup();
		let signal: AbortSignal | undefined;
		const props = renderDialog({
			runItem: (_item, ctx) =>
				new Promise<MultiTransferOutcome>(() => {
					signal = ctx.signal;
				}),
		});

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));
		await waitFor(() => expect(signal).toBeDefined());
		expect(signal?.aborted).toBe(false);

		await user.click(screen.getByTestId("close-multi-transfer-report-button"));

		expect(signal?.aborted).toBe(true);
		expect(props.onClose).toHaveBeenCalledTimes(1);
		// An abandoned run reports nothing back to the list.
		expect(props.onFinished).not.toHaveBeenCalled();
	});

	it("turns a thrown per-item failure into a failed report row", async () => {
		const user = userEvent.setup();
		const props = renderDialog({
			items: [gadgetItem("g-1")],
			runItem: async () => {
				throw axiosError(409, "GADGET_ALREADY_EXISTS", "already there");
			},
		});

		await user.click(screen.getByTestId("confirm-multi-transfer-button"));

		const rows = await screen.findAllByTestId("multi-transfer-report-row");
		expect(rows[0]).toHaveAttribute("data-status", "failed");
		expect(within(rows[0]).getByText("already there")).toBeInTheDocument();
		expect(props.onFinished).toHaveBeenCalledWith([]);
	});

	it("renders the adapter's children slot in the form phase", () => {
		renderDialog({
			children: <Text data-testid="stub-children">Extra options</Text>,
		});

		expect(screen.getByTestId("stub-children")).toBeInTheDocument();
	});

	it("renders the empty-picker copy derived from the noun, or the adapter's override", () => {
		const props = {
			host: source,
			transfer: stubTransfer({ targets: [], targetId: "", targetName: "" }),
			noun: "gadget",
			items: [gadgetItem("g-1")],
			runItem: vi.fn(),
			onFinished: vi.fn(),
			onClose: vi.fn(),
		};
		const first = render(<MultiTransferDialog {...props} />);
		expect(screen.getByTestId("move-no-targets")).toHaveTextContent(
			"No other host you can manage gadgets on.",
		);
		first.unmount();

		render(
			<MultiTransferDialog
				{...props}
				noTargetsMessage="No other host you can write to."
			/>,
		);
		expect(screen.getByTestId("move-no-targets")).toHaveTextContent(
			"No other host you can write to.",
		);
	});
});
