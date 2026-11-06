import { Td, Text, Th } from "@chakra-ui/react";
import { ReportDialog } from "@shared/components/ReportDialog";
import { formatBytes } from "@shared/utils/format";

// The structural slice of a react-query prune mutation result this adapter
// drives. Prune takes no variables, so the generalized module's `void` case
// is the whole of it: the adapter below hands the module a no-argument
// mutate and a confirm that carries nothing.
export interface PruneMutationSlice<TEntry> {
	mutate: () => void;
	isPending: boolean;
	error: unknown;
	data: TEntry[] | undefined;
	reset: () => void;
}

// The prune adapter over the shared stay-open report dialog (spec 461,
// ticket #468 generalized the skeleton out of this module). The module owns
// the plumbing — the modal shell, the reset lifecycle, the confirm/report
// footer swap, the pending wiring, the error region, and the report table's
// chrome; this adapter keeps prune's domain: the noun-derived copy, the
// entry accessors, and the reclaimed-size column. size is optional: the pods
// prune (spec #355) reports removed IDs only, and a size-less entry renders
// an em dash in the reclaimed column.
export function PruneReportDialog<
	TEntry extends { size?: number; error?: string },
>({
	mutation,
	resource,
	entryKey,
	renderLabel,
	onClose,
}: {
	mutation: PruneMutationSlice<TEntry>;
	resource: string;
	entryKey: (entry: TEntry) => string;
	renderLabel: (entry: TEntry) => string;
	onClose: () => void;
}) {
	const plural = `${resource}s`;
	const capitalized = plural.charAt(0).toUpperCase() + plural.slice(1);

	return (
		<ReportDialog<TEntry, void>
			// Prune's mutation takes no variables; the module always hands its
			// confirm variables over, so the adapter drops them here.
			mutation={{ ...mutation, mutate: () => mutation.mutate() }}
			title={`Prune unused ${plural}`}
			reportTitle="Prune report"
			confirmLabel={`Prune ${plural}`}
			confirmTestId="confirm-prune-button"
			closeTestId="close-prune-report-button"
			confirmVariables={() => undefined}
			confirmBody={
				<>
					<Text>
						All unused {plural} on this host will be removed. This cannot be
						undone.
					</Text>
					<Text mt={2} color="gray.500">
						{capitalized} that are in use by containers are safe and will not be
						touched.
					</Text>
				</>
			}
			reportHeaders={
				<>
					<Th>Removed {resource}</Th>
					<Th isNumeric>Reclaimed</Th>
				</>
			}
			reportTestId="prune-report"
			reportRowTestId="prune-report-row"
			entryKey={entryKey}
			entryFailure={(entry) => entry.error}
			renderEntry={(entry, failure) => (
				<>
					<Td fontFamily="mono">{renderLabel(entry)}</Td>
					{failure ? (
						<Td color="red.500">{failure}</Td>
					) : (
						<Td isNumeric>
							{entry.size === undefined ? "—" : formatBytes(entry.size)}
						</Td>
					)}
				</>
			)}
			emptyReport={
				<Text data-testid="prune-report-empty">
					Nothing to prune: no unused {plural} on this host.
				</Text>
			}
			onClose={onClose}
		/>
	);
}
