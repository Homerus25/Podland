import {
	type PruneMutationSlice,
	PruneReportDialog,
} from "@shared/components/PruneReportDialog";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";

// The module drives the prune mutation result the caller hands it, never
// react-query itself: the stub below is the whole interface under test.
interface StubEntry {
	id: string;
	size: number;
	error?: string;
}

function makeMutation(
	overrides: Partial<PruneMutationSlice<StubEntry>> = {},
): PruneMutationSlice<StubEntry> {
	return {
		mutate: vi.fn(),
		isPending: false,
		error: null,
		data: undefined,
		reset: vi.fn(),
		...overrides,
	};
}

function renderDialog(
	mutation = makeMutation(),
	resource = "image",
	onClose = vi.fn(),
) {
	render(
		<PruneReportDialog<StubEntry>
			mutation={mutation}
			resource={resource}
			entryKey={(entry) => entry.id}
			renderLabel={(entry) => entry.id}
			onClose={onClose}
		/>,
	);
	return onClose;
}

const REPORT: StubEntry[] = [
	{ id: "aaaa", size: 1024 },
	{ id: "bbbb", size: 0, error: "image is in use" },
];

describe("PruneReportDialog confirm state", () => {
	it("renders the noun-derived title, confirm copy, and pending-wired confirm footer, resetting on mount", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = renderDialog(mutation);

		expect(screen.getByText("Prune unused images")).toBeInTheDocument();
		expect(
			screen.getByText(
				"All unused images on this machine will be removed. This cannot be undone.",
			),
		).toBeInTheDocument();
		expect(
			screen.getByText(
				"Images that are in use by containers are safe and will not be touched.",
			),
		).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

		const confirm = screen.getByTestId("confirm-prune-button");
		expect(confirm).toHaveTextContent("Prune images");
		await user.click(confirm);

		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith();
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		expect(onClose).not.toHaveBeenCalled();
	});

	it("disables the confirm button while the mutation is pending", () => {
		renderDialog(makeMutation({ isPending: true }));

		expect(screen.getByTestId("confirm-prune-button")).toBeDisabled();
	});

	it("sends nothing on cancel", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = renderDialog(mutation);

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).not.toHaveBeenCalled();
	});

	it("resets the mutation again on unmount", () => {
		const mutation = makeMutation();
		const view = render(
			<PruneReportDialog<StubEntry>
				mutation={mutation}
				resource="image"
				entryKey={(entry) => entry.id}
				renderLabel={(entry) => entry.id}
				onClose={vi.fn()}
			/>,
		);
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		view.unmount();

		expect(mutation.reset).toHaveBeenCalledTimes(2);
	});
});

describe("PruneReportDialog report state", () => {
	it("stays open on success and swaps the footer to Close", () => {
		const onClose = vi.fn();
		const view = render(
			<PruneReportDialog<StubEntry>
				mutation={makeMutation()}
				resource="image"
				entryKey={(entry) => entry.id}
				renderLabel={(entry) => entry.id}
				onClose={onClose}
			/>,
		);
		view.rerender(
			<PruneReportDialog<StubEntry>
				mutation={makeMutation({ data: REPORT })}
				resource="image"
				entryKey={(entry) => entry.id}
				renderLabel={(entry) => entry.id}
				onClose={onClose}
			/>,
		);

		expect(screen.getByText("Prune report")).toBeInTheDocument();
		expect(screen.getByTestId("close-prune-report-button")).toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-prune-button"),
		).not.toBeInTheDocument();
		expect(onClose).not.toHaveBeenCalled();
	});

	it("renders the report table with per-entry sizes and red error cells", () => {
		renderDialog(makeMutation({ data: REPORT }));

		const report = screen.getByTestId("prune-report");
		expect(within(report).getByText("Removed image")).toBeInTheDocument();
		expect(within(report).getByText("Reclaimed")).toBeInTheDocument();
		const rows = screen.getAllByTestId("prune-report-row");
		expect(rows).toHaveLength(2);
		expect(within(rows[0]).getByText("aaaa")).toBeInTheDocument();
		expect(within(rows[0]).getByText("1 KB")).toBeInTheDocument();
		expect(within(rows[1]).getByText("bbbb")).toBeInTheDocument();
		expect(within(rows[1]).getByText("image is in use")).toBeInTheDocument();
	});

	it("renders the empty-report copy under the empty testid", () => {
		renderDialog(makeMutation({ data: [] }));

		expect(screen.getByTestId("prune-report-empty")).toHaveTextContent(
			"Nothing to prune: no unused images on this machine.",
		);
		expect(screen.queryByTestId("prune-report")).not.toBeInTheDocument();
	});
});

describe("PruneReportDialog noun-derived copy", () => {
	it("derives titles, buttons, headers, and empty copy from the resource noun", () => {
		const props = {
			mutation: makeMutation(),
			resource: "volume",
			entryKey: (entry: StubEntry) => entry.id,
			renderLabel: (entry: StubEntry) => entry.id,
			onClose: vi.fn(),
		};
		const view = render(<PruneReportDialog<StubEntry> {...props} />);

		expect(screen.getByText("Prune unused volumes")).toBeInTheDocument();
		expect(
			screen.getByText(
				"Volumes that are in use by containers are safe and will not be touched.",
			),
		).toBeInTheDocument();
		expect(screen.getByTestId("confirm-prune-button")).toHaveTextContent(
			"Prune volumes",
		);

		view.rerender(
			<PruneReportDialog<StubEntry>
				{...props}
				mutation={makeMutation({ data: [{ id: "v-1", size: 1024 }] })}
			/>,
		);
		const report = screen.getByTestId("prune-report");
		expect(within(report).getByText("Removed volume")).toBeInTheDocument();

		view.rerender(
			<PruneReportDialog<StubEntry>
				{...props}
				mutation={makeMutation({ data: [] })}
			/>,
		);
		expect(screen.getByTestId("prune-report-empty")).toHaveTextContent(
			"Nothing to prune: no unused volumes on this machine.",
		);
	});
});

describe("PruneReportDialog error region", () => {
	it("renders the default error region from the envelope and stays open", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation({
			error: axiosError(409, "PRUNE_DENIED", "Prune is denied", "req-9"),
		});
		const onClose = renderDialog(mutation);

		const alert = screen.getByRole("alert");
		expect(alert).toHaveTextContent("PRUNE_DENIED");
		expect(alert).toHaveTextContent("Prune is denied");

		await user.click(screen.getByTestId("confirm-prune-button"));
		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		expect(onClose).not.toHaveBeenCalled();
	});
});
