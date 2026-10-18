import { Box, Button, Spinner } from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";

// The structural slice of a react-query useQuery result this module
// renders: the pending flag, the error, and the raw report payload.
// Features keep their query hooks; the module never touches react-query.
export interface InspectQuerySlice {
	isPending: boolean;
	error: unknown;
	data: unknown;
}

// One owner of the raw-inspect JSON window (spec 287): the modal shell, the
// loading state, the error region, the scrollable monospace pretty-printed
// report, and the copy-to-clipboard button. Consumers render it
// conditionally, so mount is open and unmount is close — the query fires on
// open and dies on close. The feature adapter hands over the title and the
// query's structural slice; everything rendered is the module's.
export function InspectDialog({
	title,
	query,
	onClose,
}: {
	title: string;
	query: InspectQuerySlice;
	onClose: () => void;
}) {
	const report = query.data;
	const pretty = report === undefined ? "" : JSON.stringify(report, null, 2);

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={title}
			footer={
				report === undefined ? undefined : (
					<Button
						data-testid="copy-inspect-button"
						onClick={() => navigator.clipboard.writeText(pretty)}
					>
						Copy JSON
					</Button>
				)
			}
		>
			{query.isPending ? (
				<Box data-testid="inspect-dialog-loading">
					<Spinner size="sm" /> Loading inspect data…
				</Box>
			) : query.error ? (
				<ErrorDisplay
					error={getApiError(query.error)}
					data-testid="inspect-dialog-error"
				/>
			) : (
				<Box
					as="pre"
					overflow="auto"
					fontFamily="mono"
					data-testid="inspect-dialog-json"
				>
					{pretty}
				</Box>
			)}
		</Modal>
	);
}
