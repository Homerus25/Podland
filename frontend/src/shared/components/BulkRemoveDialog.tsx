import { Box, Td, Text, Th } from "@chakra-ui/react";
import {
	ReportDialog,
	type ReportMutationSlice,
} from "@shared/components/ReportDialog";
import type { BulkRemoveEntry, BulkRemoveInput } from "@shared/types/api";

// The bulk-remove dialog (spec 461, ticket #468): the one adapter over the
// shared stay-open report dialog for the seven host-scoped resources. The
// module owns the whole bulk-remove contract — the confirm copy, the
// selected-names list, the report table's headers and cells, the force
// retry, and every testid — so each list keeps only its noun and the two
// accessors that make its rows addressable. The seven per-list copies this
// replaces were byte-identical apart from those three facts.
//
// The items are the snapshot the list captured when the dialog opened, not
// the live selection: the selection clears once the run succeeds (the list
// wires `onSuccess`), and the report must still name what it reports on.
// `force` is the resource's force path, not a per-run flag — secrets have
// none, so their adapter passes `force={false}` and the report offers no
// retry at all rather than a disabled button.
export function BulkRemoveDialog<TItem>({
	items,
	noun,
	itemKey,
	itemLabel,
	mutation,
	onClose,
	onSuccess,
	force = true,
}: {
	items: TItem[];
	noun: string;
	itemKey: (item: TItem) => string;
	itemLabel: (item: TItem) => string;
	mutation: ReportMutationSlice<BulkRemoveEntry, BulkRemoveInput>;
	onClose: () => void;
	onSuccess?: () => void;
	force?: boolean;
}) {
	const ids = items.map((item) => itemKey(item));
	const count = items.length;
	const plural = `${noun}s`;
	const counted = count === 1 ? noun : plural;
	// The id→label map is built from the snapshot, so a report entry whose
	// row has already left the list still renders its name.
	const labels = new Map(items.map((item) => [itemKey(item), itemLabel(item)]));

	return (
		<ReportDialog<BulkRemoveEntry, BulkRemoveInput>
			mutation={mutation}
			title={`Remove ${count} ${counted}`}
			reportTitle="Remove report"
			confirmLabel={`Remove ${counted}`}
			confirmTestId="confirm-bulk-remove-button"
			closeTestId="close-bulk-remove-report-button"
			// The first pass is always plain; only the report's retry forces.
			confirmVariables={() => ({ ids })}
			confirmBody={
				<>
					<Text>
						Remove {count} selected {counted} from this host? This cannot be
						undone.
					</Text>
					<Box
						mt={3}
						maxH="40vh"
						overflowY="auto"
						data-testid="bulk-remove-names"
					>
						{items.map((item) => (
							<Text key={itemKey(item)} fontFamily="mono" fontSize="sm">
								{itemLabel(item)}
							</Text>
						))}
					</Box>
				</>
			}
			reportHeaders={
				<>
					<Th>{`${noun.charAt(0).toUpperCase()}${noun.slice(1)}`}</Th>
					<Th>Result</Th>
				</>
			}
			reportTestId="bulk-remove-report"
			reportRowTestId="bulk-remove-report-row"
			entryKey={(entry) => entry.id}
			entryFailure={(entry) => entry.error?.message}
			renderEntry={(entry, failure) => (
				<>
					<Td fontFamily="mono">{labels.get(entry.id) ?? entry.id}</Td>
					{failure ? <Td color="red.500">{failure}</Td> : <Td>Removed</Td>}
				</>
			)}
			emptyReport={
				<Text data-testid="bulk-remove-report-empty">
					Nothing to remove: no {plural} were selected.
				</Text>
			}
			forceRetry={
				force
					? {
							label: (failedCount) => `Force remove ${failedCount} failed`,
							testId: "force-remove-failed-button",
							variables: (failedIds) => ({ ids: failedIds, force: true }),
						}
					: undefined
			}
			onSuccess={onSuccess}
			onClose={onClose}
		/>
	);
}
