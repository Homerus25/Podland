import {
	Box,
	Button,
	Table,
	TableContainer,
	Tbody,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { type ReactNode, useEffect, useRef } from "react";

// The structural slice of a react-query mutation result this module drives:
// the ordinary mutation fields plus the success payload, whose presence is
// what flips the dialog from confirm to report. `TVariables` is the
// mutation's own input — the bulk-remove request, or `void` for a mutation
// that takes none (prune). Features keep their mutation hooks; the module
// never touches react-query.
export interface ReportMutationSlice<TEntry, TVariables = void> {
	mutate: (variables: TVariables) => void;
	isPending: boolean;
	error: unknown;
	data: TEntry[] | undefined;
	reset: () => void;
}

// The optional second pass (spec 461): a resource whose removal the server
// can refuse (in use, running) offers a retry that covers only the failed
// entries and carries force=true. The adapter owns the words, the testid,
// and the variables; the module owns when the button exists at all — only
// when the report actually holds failures. A resource with no force path
// (secrets) passes no `forceRetry`, so "no retry" is the absence of the
// seam, never a disabled button.
export interface ReportForceRetry<TVariables> {
	label: (failedCount: number) => string;
	testId: string;
	variables: (failedIds: string[]) => TVariables;
}

// One owner of the stay-open report dialog: the skeleton every surface that
// renders a mutation's success payload in place shares (prune, and the
// bulk-remove flow of spec 461). Consumers render it conditionally, so mount
// is open and unmount is close; the module runs reset() on both ends, owns
// the confirm/report footer swap (the whole point is staying open to render
// what the mutation returned), the pending wiring, the in-modal error
// region, the report table's chrome, and the force-retry button. Everything
// per-surface is handed over as data or as a render seam: the copy, the
// confirm body, the report headers, the entry key, the failure predicate,
// the entry cells, and the empty-report node. `renderEntry` receives the
// failure the module already derived, so the adapter never re-decides what
// counts as a failure and the retry can never disagree with the table.
//
// `onSuccess` is the run's own seam, fired once per completed run — the
// first pass and each retry — the moment the report arrives. It is what
// "after the run" means for a caller whose side effect must not wait for
// the dialog to close (the bulk-remove selection clears here, so a failed
// run keeps it).
export function ReportDialog<TEntry, TVariables = void>({
	mutation,
	title,
	reportTitle,
	confirmLabel,
	confirmTestId,
	closeTestId,
	confirmVariables,
	confirmBody,
	reportHeaders,
	reportTestId,
	reportRowTestId,
	entryKey,
	entryFailure,
	renderEntry,
	emptyReport,
	forceRetry,
	onSuccess,
	onClose,
}: {
	mutation: ReportMutationSlice<TEntry, TVariables>;
	title: string;
	reportTitle: string;
	confirmLabel: string;
	confirmTestId: string;
	closeTestId: string;
	// The first pass's variables: always the plain request, never forced.
	confirmVariables: () => TVariables;
	confirmBody: ReactNode;
	reportHeaders: ReactNode;
	reportTestId: string;
	reportRowTestId: string;
	entryKey: (entry: TEntry) => string;
	entryFailure: (entry: TEntry) => string | undefined;
	renderEntry: (entry: TEntry, failure: string | undefined) => ReactNode;
	emptyReport: ReactNode;
	forceRetry?: ReportForceRetry<TVariables>;
	onSuccess?: () => void;
	onClose: () => void;
}) {
	const { reset } = mutation;
	useEffect(() => {
		reset();
		return () => reset();
	}, [reset]);

	// The success seam fires once per completed run. The ref starts at the
	// first render's payload so a stale result cannot fire it on mount (the
	// reset above clears the mutation, but this effect's closure still sees
	// the render's value), and it advances on every new report — the retry
	// is a run too.
	const reportedRef = useRef<TEntry[] | undefined>(mutation.data);
	useEffect(() => {
		if (mutation.data === undefined || reportedRef.current === mutation.data) {
			return;
		}
		reportedRef.current = mutation.data;
		onSuccess?.();
	}, [mutation.data, onSuccess]);

	const report = mutation.data;
	// The retry covers exactly the entries the report marks as failures, in
	// report order — the successes are already gone and are never re-sent.
	const failedIds = report
		? report.filter((entry) => entryFailure(entry) !== undefined).map(entryKey)
		: [];
	const retry = forceRetry !== undefined && failedIds.length > 0;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={report ? reportTitle : title}
			footer={
				report ? (
					<>
						{retry ? (
							<Button
								colorScheme="red"
								variant="outline"
								onClick={() => mutation.mutate(forceRetry.variables(failedIds))}
								isLoading={mutation.isPending}
								data-testid={forceRetry.testId}
							>
								{forceRetry.label(failedIds.length)}
							</Button>
						) : null}
						<Button onClick={onClose} data-testid={closeTestId}>
							Close
						</Button>
					</>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme="red"
							onClick={() => mutation.mutate(confirmVariables())}
							isLoading={mutation.isPending}
							data-testid={confirmTestId}
						>
							{confirmLabel}
						</Button>
					</>
				)
			}
		>
			{mutation.error ? (
				<Box mt={2}>
					<ErrorDisplay error={getApiError(mutation.error)} />
				</Box>
			) : null}
			{report ? (
				report.length === 0 ? (
					emptyReport
				) : (
					<TableContainer>
						<Table data-testid={reportTestId} size="sm">
							<Thead>
								<Tr>{reportHeaders}</Tr>
							</Thead>
							<Tbody>
								{report.map((entry) => (
									<Tr key={entryKey(entry)} data-testid={reportRowTestId}>
										{renderEntry(entry, entryFailure(entry))}
									</Tr>
								))}
							</Tbody>
						</Table>
					</TableContainer>
				)
			) : (
				confirmBody
			)}
		</Modal>
	);
}
