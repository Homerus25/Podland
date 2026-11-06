import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import type { ReportMutationSlice } from "@shared/components/ReportDialog";
import type { BulkRemoveEntry, BulkRemoveInput } from "@shared/types/api";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

// The one bulk-remove adapter over ReportDialog (spec 461, ticket #468). The
// seven lists hand it a noun and two accessors and nothing else, so the copy,
// the report chrome, the force retry and the testids are pinned here once
// instead of seven times. The skeleton's own behaviour (stay-open report,
// footer swap, error region) is pinned in the ReportDialog suite; this suite
// keeps what the adapter decides.

interface StubItem {
	name: string;
	label: string;
}

const VOLUME_A: StubItem = { name: "volume-a", label: "volume-a" };
const VOLUME_B: StubItem = { name: "volume-b", label: "volume-b" };

function makeMutation(
	overrides: Partial<
		ReportMutationSlice<BulkRemoveEntry, BulkRemoveInput>
	> = {},
): ReportMutationSlice<BulkRemoveEntry, BulkRemoveInput> {
	return {
		mutate: vi.fn(),
		isPending: false,
		error: null,
		data: undefined,
		reset: vi.fn(),
		...overrides,
	};
}

function renderDialog({
	items = [VOLUME_A, VOLUME_B],
	mutation = makeMutation(),
	force = true,
	onSuccess,
}: {
	items?: StubItem[];
	mutation?: ReportMutationSlice<BulkRemoveEntry, BulkRemoveInput>;
	force?: boolean;
	onSuccess?: () => void;
} = {}) {
	render(
		<BulkRemoveDialog<StubItem>
			items={items}
			noun="volume"
			itemKey={(item) => item.name}
			itemLabel={(item) => item.label}
			mutation={mutation}
			onClose={vi.fn()}
			onSuccess={onSuccess}
			force={force}
		/>,
	);
}

describe("BulkRemoveDialog copy", () => {
	it("counts one item in the singular", () => {
		renderDialog({ items: [VOLUME_A] });

		expect(screen.getByText("Remove 1 volume")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-bulk-remove-button")).toHaveTextContent(
			"Remove volume",
		);
		expect(
			screen.getByText(
				"Remove 1 selected volume from this host? This cannot be undone.",
			),
		).toBeInTheDocument();
	});

	it("counts several items in the plural and lists their labels", () => {
		renderDialog();

		expect(screen.getByText("Remove 2 volumes")).toBeInTheDocument();
		expect(screen.getByTestId("confirm-bulk-remove-button")).toHaveTextContent(
			"Remove volumes",
		);
		const names = screen.getByTestId("bulk-remove-names");
		expect(within(names).getByText("volume-a")).toBeInTheDocument();
		expect(within(names).getByText("volume-b")).toBeInTheDocument();
	});

	it("sends the plain request on confirm, never a forced one", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		renderDialog({ mutation });

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));

		expect(mutation.mutate).toHaveBeenCalledWith({
			ids: ["volume-a", "volume-b"],
		});
	});
});

describe("BulkRemoveDialog report", () => {
	it("capitalizes the noun as the report's first header", () => {
		renderDialog({
			mutation: makeMutation({ data: [{ id: "volume-a", success: true }] }),
		});

		const report = screen.getByTestId("bulk-remove-report");
		expect(within(report).getByText("Volume")).toBeInTheDocument();
		expect(within(report).getByText("Result")).toBeInTheDocument();
	});

	it("names each entry from the snapshot, falling back to the id", () => {
		renderDialog({
			items: [{ name: "volume-a", label: "the first volume" }],
			mutation: makeMutation({
				data: [
					{ id: "volume-a", success: true },
					{
						id: "volume-gone",
						success: false,
						error: { code: "VOLUME_IN_USE", message: "in use" },
					},
				],
			}),
		});

		const rows = screen.getAllByTestId("bulk-remove-report-row");
		expect(within(rows[0]).getByText("the first volume")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		// The row left the list before the report arrived; the id still names it.
		expect(within(rows[1]).getByText("volume-gone")).toBeInTheDocument();
		expect(within(rows[1]).getByText("in use")).toBeInTheDocument();
	});

	it("offers the forced retry for exactly the failed entries", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation({
			data: [
				{ id: "volume-a", success: true },
				{
					id: "volume-b",
					success: false,
					error: { code: "VOLUME_IN_USE", message: "in use" },
				},
			],
		});
		renderDialog({ mutation });

		const retry = screen.getByTestId("force-remove-failed-button");
		expect(retry).toHaveTextContent("Force remove 1 failed");
		await user.click(retry);

		expect(mutation.mutate).toHaveBeenCalledWith({
			ids: ["volume-b"],
			force: true,
		});
	});

	it("offers no retry when the resource has no force path, and still reports the failure", () => {
		renderDialog({
			force: false,
			mutation: makeMutation({
				data: [
					{
						id: "volume-a",
						success: false,
						error: { code: "SECRET_IN_USE", message: "in use" },
					},
				],
			}),
		});

		expect(
			screen.queryByTestId("force-remove-failed-button"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("close-bulk-remove-report-button"),
		).toBeInTheDocument();
		expect(screen.getByText("in use")).toBeInTheDocument();
	});

	it("passes the success seam through to the skeleton", () => {
		const onSuccess = vi.fn();
		renderDialog({
			mutation: makeMutation({ data: [{ id: "volume-a", success: true }] }),
			onSuccess,
		});

		// A stale report on mount is not a run: the seam stays quiet.
		expect(onSuccess).not.toHaveBeenCalled();
	});
});
