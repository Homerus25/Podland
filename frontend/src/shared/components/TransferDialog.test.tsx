import { Text } from "@chakra-ui/react";
import {
	TransferDialog,
	type TransferPreviewSlice,
	type TransferState,
	useTransferState,
} from "@shared/components/TransferDialog";
import { getApiError } from "@shared/utils/api";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";
import { hostFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";

const source = hostFixture();
const staging = hostFixture({ id: "m-2", name: "staging-01" });
const backup = hostFixture({ id: "m-4", name: "backup-01" });
const readOnly = hostFixture({
	id: "m-3",
	name: "laptop-01",
	permission: "read",
	isOwner: false,
});

// The skeleton's host facts arrive through the feature hook the preset
// composes; the stub stands in for the query so this suite pins the dialog,
// not the transport. The fixtures are read inside the stubbed hook body, so
// the mock's hoist still sees them initialized.
vi.mock("@features/hosts/hooks/useHosts", () => ({
	useHosts: () => ({ data: [source, staging, readOnly, backup] }),
}));

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

// The preset as an adapter rides it: the real state hook over the stubbed
// hosts, so target and mode interactions run through real state.
function TransferHarness(
	props: Omit<Parameters<typeof TransferDialog>[0], "host" | "transfer">,
) {
	const transfer = useTransferState(source, "manage-volumes");
	return <TransferDialog host={source} transfer={transfer} {...props} />;
}

function renderHarness(
	overrides: Partial<Parameters<typeof TransferDialog>[0]> = {},
) {
	const props = {
		preview: stubPreview(),
		noun: "gadget",
		itemName: "g-1",
		isPending: false,
		error: null,
		report: null,
		onConfirm: vi.fn(),
		onClose: vi.fn(),
		...overrides,
	};
	const view = render(<TransferHarness {...props} />);
	return { ...props, unmount: view.unmount };
}

describe("useTransferState", () => {
	it("filters hosts by capability, source excluded, defaulting to the first target", () => {
		const { result } = renderHookWithQuery(() =>
			useTransferState(source, "manage-volumes"),
		);

		expect(result.current.targets.map((target) => target.id)).toEqual([
			"m-2",
			"m-4",
		]);
		expect(result.current.targetId).toBe("m-2");
		expect(result.current.targetName).toBe("staging-01");
		expect(result.current.mode).toBe("move");
	});

	it("carries a chosen target and a mode switch", () => {
		const { result } = renderHookWithQuery(() =>
			useTransferState(source, "manage-volumes"),
		);

		act(() => {
			result.current.selectTarget("m-4");
		});
		expect(result.current.targetId).toBe("m-4");
		expect(result.current.targetName).toBe("backup-01");

		act(() => {
			result.current.setMode("copy");
		});
		expect(result.current.mode).toBe("copy");
	});
});

describe("TransferDialog skeleton", () => {
	it("renders the target select, the mode radio, and the noun-derived confirm", async () => {
		renderHarness();
		const user = userEvent.setup();

		const select = screen.getByTestId("move-target-select");
		const names = within(select)
			.getAllByRole("option")
			.map((option) => option.textContent);
		expect(names).toEqual(["staging-01", "backup-01"]);

		expect(
			screen.getByRole("radio", { name: "Move (remove from this host)" }),
		).toBeChecked();
		expect(
			screen.getByRole("radio", { name: "Copy (keep this host's gadget)" }),
		).not.toBeChecked();
		expect(screen.getByTestId("confirm-move-button")).toHaveTextContent(
			"Move gadget",
		);

		// The mode switch retells the confirm in copy vocabulary.
		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's gadget)" }),
		);
		expect(screen.getByTestId("confirm-move-button")).toHaveTextContent(
			"Copy gadget",
		);
	});

	it("hides the mode radio and pins the confirm to Copy when the mode is pinned", () => {
		renderHarness({ pinnedMode: "copy" });

		expect(screen.queryByTestId("move-mode-radio")).not.toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toHaveTextContent(
			"Copy gadget",
		);
		expect(screen.getByTestId("modal")).toHaveTextContent("Copy g-1");
	});

	it("sends confirm once clicked, and nothing on cancel", async () => {
		const props = renderHarness();
		const user = userEvent.setup();

		await user.click(screen.getByTestId("confirm-move-button"));
		expect(props.onConfirm).toHaveBeenCalledTimes(1);

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(props.onClose).toHaveBeenCalledTimes(1);
		expect(props.onConfirm).toHaveBeenCalledTimes(1);
	});

	it("keeps confirm disabled while the preview is pending", () => {
		renderHarness({ preview: stubPreview({ isPending: true }) });

		expect(screen.getByTestId("move-preview-pending")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("shows a failed preview and keeps confirm disabled", () => {
		renderHarness({
			preview: stubPreview({
				isError: true,
				error: axiosError(403, "ACCESS_DENIED", "Access denied", "req-71"),
			}),
		});

		expect(screen.getByTestId("move-preview-error")).toHaveTextContent(
			"ACCESS_DENIED",
		);
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("blocks confirm on a collision with the noun-derived warning", () => {
		renderHarness({ preview: stubPreview({ data: { collision: true } }) });

		const warning = screen.getByTestId("move-collision-warning");
		expect(warning).toHaveTextContent("A gadget named “g-1”");
		expect(warning).toHaveTextContent("already exists on staging-01");
		expect(warning).toHaveTextContent("remove it there first");
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("keeps confirm disabled while the transfer is pending or the adapter refuses", () => {
		const pending = renderHarness({ isPending: true });
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
		pending.unmount();

		renderHarness({ confirmDisabled: true });
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();
	});

	it("renders the transfer error with the hint seam under it", () => {
		renderHarness({
			error: getApiError(
				axiosError(409, "GADGET_REFUSED", "The gadget refused"),
			),
			renderHints: (error) =>
				error.code === "GADGET_REFUSED" ? (
					<Text data-testid="stub-hint">Tick the box and confirm again</Text>
				) : null,
		});

		expect(screen.getByTestId("mutation-dialog-error")).toHaveTextContent(
			"GADGET_REFUSED",
		);
		expect(screen.getByTestId("stub-hint")).toBeInTheDocument();
	});

	it("swaps the footer to Close on the stay-open report and narrates in place", async () => {
		const props = renderHarness({
			report: <Text data-testid="stub-report">Moved g-1</Text>,
		});
		const user = userEvent.setup();

		expect(screen.getByTestId("stub-report")).toBeInTheDocument();
		expect(screen.queryByTestId("move-target-select")).not.toBeInTheDocument();
		expect(screen.queryByTestId("confirm-move-button")).not.toBeInTheDocument();

		await user.click(screen.getByTestId("close-move-report-button"));
		expect(props.onClose).toHaveBeenCalledTimes(1);
	});

	it("lists referencing quadlets with the dangling note in move mode only", async () => {
		renderHarness({
			referencingQuadlets: [
				{ name: "web.container" },
				{ name: "db.container" },
			],
		});
		const user = userEvent.setup();

		const warning = screen.getByTestId("move-referencing-warning");
		expect(warning).toHaveTextContent("web.container");
		expect(warning).toHaveTextContent("db.container");
		expect(warning).toHaveTextContent("keep pointing at prod-web-01");
		expect(warning).toHaveTextContent("references dangle");

		await user.click(
			screen.getByRole("radio", { name: "Copy (keep this host's gadget)" }),
		);
		expect(
			screen.getByTestId("move-referencing-warning"),
		).not.toHaveTextContent("references dangle");
	});

	it("renders the warnings slot above the referencing warning and children below", () => {
		render(
			<TransferDialog
				host={source}
				transfer={stubTransfer()}
				preview={stubPreview()}
				noun="gadget"
				itemName="g-1"
				isPending={false}
				error={null}
				report={null}
				warnings={<Text data-testid="stub-warnings">Warnings</Text>}
				referencingQuadlets={[{ name: "web.container" }]}
				onConfirm={vi.fn()}
				onClose={vi.fn()}
			>
				<Text data-testid="stub-children">Children</Text>
			</TransferDialog>,
		);

		const warnings = screen.getByTestId("stub-warnings");
		const referencing = screen.getByTestId("move-referencing-warning");
		const children = screen.getByTestId("stub-children");
		expect(
			warnings.compareDocumentPosition(referencing) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		expect(
			referencing.compareDocumentPosition(children) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
	});

	it("renders the empty-picker copy derived from the noun, or the adapter's override", () => {
		const empty = stubTransfer({
			targets: [],
			targetId: "",
			targetName: "",
		});
		const props = {
			host: source,
			transfer: empty,
			preview: stubPreview(),
			noun: "gadget",
			itemName: "g-1",
			isPending: false,
			error: null,
			report: null,
			onConfirm: vi.fn(),
			onClose: vi.fn(),
		};
		const first = render(<TransferDialog {...props} />);
		expect(screen.getByTestId("move-no-targets")).toHaveTextContent(
			"No other host you can manage gadgets on.",
		);
		// No writable target means nothing to confirm against.
		expect(screen.getByTestId("confirm-move-button")).toBeDisabled();

		first.unmount();
		render(
			<TransferDialog
				{...props}
				noTargetsMessage="No other host you can write to."
			/>,
		);
		expect(screen.getByTestId("move-no-targets")).toHaveTextContent(
			"No other host you can write to.",
		);
	});

	it("runs the reset seam on mount and unmount", () => {
		const reset = vi.fn();
		const view = render(
			<TransferDialog
				host={source}
				transfer={stubTransfer()}
				preview={stubPreview()}
				noun="gadget"
				itemName="g-1"
				isPending={false}
				error={null}
				report={null}
				reset={reset}
				onConfirm={vi.fn()}
				onClose={vi.fn()}
			/>,
		);
		expect(reset).toHaveBeenCalledTimes(1);

		view.unmount();
		expect(reset).toHaveBeenCalledTimes(2);
	});
});
