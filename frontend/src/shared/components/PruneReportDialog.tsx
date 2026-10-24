import {
	Box,
	Button,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";
import { useEffect } from "react";

// The structural slice of a react-query prune mutation result this module
// drives: the ordinary mutation fields plus the success payload, whose
// presence is what flips the dialog from confirm to report. Features keep
// their mutation hooks; the module never touches react-query.
export interface PruneMutationSlice<TEntry> {
	mutate: () => void;
	isPending: boolean;
	error: unknown;
	data: TEntry[] | undefined;
	reset: () => void;
}

// One owner of the stay-open report dialog that every prune feature used
// to hand-roll twice. Consumers render it conditionally, so mount is open
// and unmount is close; the module runs reset() on both ends, owns the
// confirm/report footer swap (a prune dialog's whole point is staying open
// to render what the mutation returned), the report table, the empty-report
// and confirm copy derived from the singular `resource` noun, and the
// in-modal error region. Everything per-feature is handed over as data:
// the noun, the entry key, and the label renderer. size is optional: the
// pods prune (spec #355) reports removed IDs only, and a size-less entry
// renders an em dash in the reclaimed column.
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
	const { reset } = mutation;
	useEffect(() => {
		reset();
		return () => reset();
	}, [reset]);

	const plural = `${resource}s`;
	const capitalized = plural.charAt(0).toUpperCase() + plural.slice(1);
	const report = mutation.data;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={report ? "Prune report" : `Prune unused ${plural}`}
			footer={
				report ? (
					<Button onClick={onClose} data-testid="close-prune-report-button">
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme="red"
							onClick={() => mutation.mutate()}
							isLoading={mutation.isPending}
							data-testid="confirm-prune-button"
						>
							Prune {plural}
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
					<Text data-testid="prune-report-empty">
						Nothing to prune: no unused {plural} on this machine.
					</Text>
				) : (
					<TableContainer>
						<Table data-testid="prune-report" size="sm">
							<Thead>
								<Tr>
									<Th>Removed {resource}</Th>
									<Th isNumeric>Reclaimed</Th>
								</Tr>
							</Thead>
							<Tbody>
								{report.map((entry) => (
									<Tr key={entryKey(entry)} data-testid="prune-report-row">
										<Td fontFamily="mono">{renderLabel(entry)}</Td>
										{entry.error ? (
											<Td color="red.500">{entry.error}</Td>
										) : (
											<Td isNumeric>
												{entry.size === undefined
													? "—"
													: formatBytes(entry.size)}
											</Td>
										)}
									</Tr>
								))}
							</Tbody>
						</Table>
					</TableContainer>
				)
			) : (
				<>
					<Text>
						All unused {plural} on this machine will be removed. This cannot be
						undone.
					</Text>
					<Text mt={2} color="gray.500">
						{capitalized} that are in use by containers are safe and will not be
						touched.
					</Text>
				</>
			)}
		</Modal>
	);
}
