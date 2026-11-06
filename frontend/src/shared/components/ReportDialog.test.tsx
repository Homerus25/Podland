import { Td, Text, Th } from "@chakra-ui/react";
import {
	ReportDialog,
	type ReportForceRetry,
	type ReportMutationSlice,
} from "@shared/components/ReportDialog";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { axiosError } from "@/test/errors";

// The module drives the mutation result the caller hands it, never
// react-query itself: the stub below is the whole interface under test. The
// entry and variable shapes are the bulk-remove ones (an id plus an optional
// code/message failure, and an ids-plus-force request) because that is the
// surface the generalized skeleton was extracted for; prune's own suite
// keeps pinning the no-variable adapter.
interface StubEntry {
	id: string;
	error?: { code: string; message: string };
}

interface StubVariables {
	ids: string[];
	force: boolean;
}

function makeMutation(
	overrides: Partial<ReportMutationSlice<StubEntry, StubVariables>> = {},
): ReportMutationSlice<StubEntry, StubVariables> {
	return {
		mutate: vi.fn(),
		isPending: false,
		error: null,
		data: undefined,
		reset: vi.fn(),
		...overrides,
	};
}

const FORCE_RETRY: ReportForceRetry<StubVariables> = {
	label: (failedCount) => `Force remove ${failedCount} failed`,
	testId: "force-remove-failed-button",
	variables: (failedIds) => ({ ids: failedIds, force: true }),
};

const REPORT: StubEntry[] = [
	{ id: "volume-a" },
	{
		id: "volume-b",
		error: {
			code: "VOLUME_IN_USE",
			message: "volume is in use by container abc",
		},
	},
	{
		id: "volume-c",
		error: {
			code: "VOLUME_IN_USE",
			message: "volume is in use by container def",
		},
	},
];

function renderDialog({
	mutation = makeMutation(),
	onClose = vi.fn(),
	forceRetry = FORCE_RETRY,
	onSuccess,
}: {
	mutation?: ReportMutationSlice<StubEntry, StubVariables>;
	onClose?: () => void;
	// null is the resource with no force path at all (secrets): the seam is
	// absent, not disabled.
	forceRetry?: ReportForceRetry<StubVariables> | null;
	onSuccess?: () => void;
} = {}) {
	render(dialogElement({ mutation, onClose, forceRetry, onSuccess }));
	return onClose;
}

// The element itself, so a test can rerender the same dialog with a new
// mutation result — the only way to model "the run came back" without
// react-query.
function dialogElement({
	mutation = makeMutation(),
	onClose = vi.fn(),
	forceRetry = FORCE_RETRY,
	onSuccess,
}: {
	mutation?: ReportMutationSlice<StubEntry, StubVariables>;
	onClose?: () => void;
	forceRetry?: ReportForceRetry<StubVariables> | null;
	onSuccess?: () => void;
} = {}) {
	return (
		<ReportDialog<StubEntry, StubVariables>
			mutation={mutation}
			title="Remove 2 volumes"
			reportTitle="Remove report"
			confirmLabel="Remove volumes"
			confirmTestId="confirm-bulk-remove-button"
			closeTestId="close-bulk-remove-report-button"
			confirmVariables={() => ({ ids: ["volume-a", "volume-b"], force: false })}
			confirmBody={<Text>Two volumes are about to go.</Text>}
			reportHeaders={
				<>
					<Th>Volume</Th>
					<Th>Result</Th>
				</>
			}
			reportTestId="bulk-remove-report"
			reportRowTestId="bulk-remove-report-row"
			entryKey={(entry) => entry.id}
			entryFailure={(entry) => entry.error?.message}
			renderEntry={(entry, failure) => (
				<>
					<Td fontFamily="mono">{entry.id}</Td>
					{failure ? <Td color="red.500">{failure}</Td> : <Td>Removed</Td>}
				</>
			)}
			emptyReport={
				<Text data-testid="bulk-remove-report-empty">Nothing to remove.</Text>
			}
			forceRetry={forceRetry ?? undefined}
			onSuccess={onSuccess}
			onClose={onClose}
		/>
	);
}

describe("ReportDialog confirm state", () => {
	it("renders the adapter's title and confirm body, and sends the plain variables on confirm", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = renderDialog({ mutation });

		expect(screen.getByText("Remove 2 volumes")).toBeInTheDocument();
		expect(
			screen.getByText("Two volumes are about to go."),
		).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

		const confirm = screen.getByTestId("confirm-bulk-remove-button");
		expect(confirm).toHaveTextContent("Remove volumes");
		await user.click(confirm);

		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		// The first pass is always the plain request: the adapter's
		// confirmVariables carries no force.
		expect(mutation.mutate).toHaveBeenCalledWith({
			ids: ["volume-a", "volume-b"],
			force: false,
		});
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		expect(onClose).not.toHaveBeenCalled();
	});

	it("disables the confirm button while the mutation is pending", () => {
		renderDialog({ mutation: makeMutation({ isPending: true }) });

		expect(screen.getByTestId("confirm-bulk-remove-button")).toBeDisabled();
	});

	it("sends nothing on cancel", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation();
		const onClose = renderDialog({ mutation });

		await user.click(screen.getByRole("button", { name: "Cancel" }));

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).not.toHaveBeenCalled();
	});

	it("resets the mutation again on unmount", () => {
		const mutation = makeMutation();
		const view = render(
			<ReportDialog<StubEntry, StubVariables>
				mutation={mutation}
				title="Remove 2 volumes"
				reportTitle="Remove report"
				confirmLabel="Remove volumes"
				confirmTestId="confirm-bulk-remove-button"
				closeTestId="close-bulk-remove-report-button"
				confirmVariables={() => ({ ids: ["volume-a"], force: false })}
				confirmBody={<Text>Two volumes are about to go.</Text>}
				reportHeaders={<Th>Volume</Th>}
				reportTestId="bulk-remove-report"
				reportRowTestId="bulk-remove-report-row"
				entryKey={(entry) => entry.id}
				entryFailure={(entry) => entry.error?.message}
				renderEntry={(entry) => <Td>{entry.id}</Td>}
				emptyReport={<Text>Nothing to remove.</Text>}
				onClose={vi.fn()}
			/>,
		);
		expect(mutation.reset).toHaveBeenCalledTimes(1);
		view.unmount();

		expect(mutation.reset).toHaveBeenCalledTimes(2);
	});
});

describe("ReportDialog report state", () => {
	it("stays open on success and swaps the footer to Close", () => {
		const onClose = vi.fn();
		const view = render(
			<ReportDialog<StubEntry, StubVariables>
				mutation={makeMutation()}
				title="Remove 2 volumes"
				reportTitle="Remove report"
				confirmLabel="Remove volumes"
				confirmTestId="confirm-bulk-remove-button"
				closeTestId="close-bulk-remove-report-button"
				confirmVariables={() => ({ ids: ["volume-a"], force: false })}
				confirmBody={<Text>Two volumes are about to go.</Text>}
				reportHeaders={<Th>Volume</Th>}
				reportTestId="bulk-remove-report"
				reportRowTestId="bulk-remove-report-row"
				entryKey={(entry) => entry.id}
				entryFailure={(entry) => entry.error?.message}
				renderEntry={(entry) => <Td>{entry.id}</Td>}
				emptyReport={<Text>Nothing to remove.</Text>}
				onClose={onClose}
			/>,
		);
		view.rerender(
			<ReportDialog<StubEntry, StubVariables>
				mutation={makeMutation({ data: REPORT })}
				title="Remove 2 volumes"
				reportTitle="Remove report"
				confirmLabel="Remove volumes"
				confirmTestId="confirm-bulk-remove-button"
				closeTestId="close-bulk-remove-report-button"
				confirmVariables={() => ({ ids: ["volume-a"], force: false })}
				confirmBody={<Text>Two volumes are about to go.</Text>}
				reportHeaders={<Th>Volume</Th>}
				reportTestId="bulk-remove-report"
				reportRowTestId="bulk-remove-report-row"
				entryKey={(entry) => entry.id}
				entryFailure={(entry) => entry.error?.message}
				renderEntry={(entry) => <Td>{entry.id}</Td>}
				emptyReport={<Text>Nothing to remove.</Text>}
				onClose={onClose}
			/>,
		);

		expect(screen.getByText("Remove report")).toBeInTheDocument();
		expect(
			screen.getByTestId("close-bulk-remove-report-button"),
		).toBeInTheDocument();
		expect(
			screen.queryByTestId("confirm-bulk-remove-button"),
		).not.toBeInTheDocument();
		expect(onClose).not.toHaveBeenCalled();
	});

	it("renders the adapter's headers and per-entry cells, successes and failures alike", () => {
		renderDialog({ mutation: makeMutation({ data: REPORT }) });

		const report = screen.getByTestId("bulk-remove-report");
		expect(within(report).getByText("Volume")).toBeInTheDocument();
		expect(within(report).getByText("Result")).toBeInTheDocument();

		const rows = screen.getAllByTestId("bulk-remove-report-row");
		expect(rows).toHaveLength(3);
		expect(within(rows[0]).getByText("volume-a")).toBeInTheDocument();
		expect(within(rows[0]).getByText("Removed")).toBeInTheDocument();
		expect(within(rows[1]).getByText("volume-b")).toBeInTheDocument();
		expect(
			within(rows[1]).getByText("volume is in use by container abc"),
		).toBeInTheDocument();
		expect(within(rows[2]).getByText("volume-c")).toBeInTheDocument();
		expect(
			within(rows[2]).getByText("volume is in use by container def"),
		).toBeInTheDocument();
	});

	it("renders the adapter's empty-report node when the report is empty", () => {
		renderDialog({ mutation: makeMutation({ data: [] }) });

		expect(screen.getByTestId("bulk-remove-report-empty")).toHaveTextContent(
			"Nothing to remove.",
		);
		expect(screen.queryByTestId("bulk-remove-report")).not.toBeInTheDocument();
	});
});

// The run's own seam: "after the run" is the report arriving, not the dialog
// closing, so a caller's side effect (the bulk-remove selection clearing)
// happens while the report is still on screen and survives a failed run.
describe("ReportDialog success seam", () => {
	it("fires once when the run's report arrives, while the dialog stays open", () => {
		const onSuccess = vi.fn();
		const view = render(dialogElement({ onSuccess }));

		expect(onSuccess).not.toHaveBeenCalled();

		view.rerender(
			dialogElement({ mutation: makeMutation({ data: REPORT }), onSuccess }),
		);

		expect(onSuccess).toHaveBeenCalledTimes(1);
		expect(screen.getByTestId("bulk-remove-report")).toBeInTheDocument();
	});

	it("does not fire on mount when the mutation already holds a stale report", () => {
		const onSuccess = vi.fn();
		renderDialog({ mutation: makeMutation({ data: REPORT }), onSuccess });

		expect(onSuccess).not.toHaveBeenCalled();
	});

	it("fires again for the force retry's own report", () => {
		const onSuccess = vi.fn();
		const view = render(
			dialogElement({ mutation: makeMutation({ data: REPORT }), onSuccess }),
		);
		expect(onSuccess).not.toHaveBeenCalled();

		view.rerender(
			dialogElement({
				mutation: makeMutation({ data: [{ id: "volume-b" }] }),
				onSuccess,
			}),
		);

		expect(onSuccess).toHaveBeenCalledTimes(1);
	});
});

describe("ReportDialog force retry", () => {
	it("offers no retry when every entry succeeded", () => {
		renderDialog({
			mutation: makeMutation({
				data: [{ id: "volume-a" }, { id: "volume-b" }],
			}),
		});

		expect(
			screen.queryByTestId("force-remove-failed-button"),
		).not.toBeInTheDocument();
		expect(
			screen.getByTestId("close-bulk-remove-report-button"),
		).toBeInTheDocument();
	});

	it("offers no retry when the resource has no force path, and still reports the failures", () => {
		renderDialog({
			mutation: makeMutation({ data: REPORT }),
			forceRetry: null,
		});

		expect(
			screen.queryByTestId("force-remove-failed-button"),
		).not.toBeInTheDocument();
		expect(
			screen.getByText("volume is in use by container abc"),
		).toBeInTheDocument();
		expect(
			screen.getByTestId("close-bulk-remove-report-button"),
		).toBeInTheDocument();
	});

	it("retries only the failed ids, with force=true", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation({ data: REPORT });
		renderDialog({ mutation });

		const retry = screen.getByTestId("force-remove-failed-button");
		expect(retry).toHaveTextContent("Force remove 2 failed");
		await user.click(retry);

		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		expect(mutation.mutate).toHaveBeenCalledWith({
			ids: ["volume-b", "volume-c"],
			force: true,
		});
	});

	it("disables the retry while the retry is pending", () => {
		renderDialog({
			mutation: makeMutation({ data: REPORT, isPending: true }),
		});

		expect(screen.getByTestId("force-remove-failed-button")).toBeDisabled();
	});
});

describe("ReportDialog error region", () => {
	it("renders the default error region from the envelope and stays open", async () => {
		const user = userEvent.setup();
		const mutation = makeMutation({
			error: axiosError(
				403,
				"ACCESS_DENIED",
				"Write access is required",
				"req-9",
			),
		});
		const onClose = renderDialog({ mutation });

		const alert = screen.getByRole("alert");
		expect(alert).toHaveTextContent("ACCESS_DENIED");
		expect(alert).toHaveTextContent("Write access is required");

		await user.click(screen.getByTestId("confirm-bulk-remove-button"));
		expect(mutation.mutate).toHaveBeenCalledTimes(1);
		expect(onClose).not.toHaveBeenCalled();
	});
});
