import { Box, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import { displayTags } from "@features/images/components/imageDisplayTags";
import {
	type ImageCopyProgress,
	useImageCopyStream,
} from "@features/images/hooks/useImageCopyStream";
import { useImageCopyPreview } from "@features/images/hooks/useImages";
import type { ImageSummary } from "@features/images/services/imagesApi";
import {
	TransferDialog,
	type TransferPreviewSlice,
	useTransferState,
} from "@shared/components/TransferDialog";
import {
	TransferReport,
	transferProgressLabel,
} from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";

// The one refusal the dialog can make actionable (issue 376 over 375's
// wire): the archive crossed the server's transfer bound. It rides the
// stream's terminal error event.
const IMAGE_TRANSFER_TOO_LARGE_CODE = "IMAGE_TRANSFER_TOO_LARGE";

// Live archive progress: one line, updated in place as the stream
// advances. The phase-to-label mapping is the shared transfer-presentation
// module's (candidate C8); the entry passes directly.
function TransferProgress({ progress }: { progress: ImageCopyProgress }) {
	return (
		<HStack spacing={2} data-testid="move-progress">
			<Spinner size="sm" />
			<Text>Copying image archive…</Text>
			<Text color="gray.500">{transferProgressLabel(progress)}</Text>
		</HStack>
	);
}

// One dialog for the whole image copy (issue 376): the shared
// transfer-dialog preset carries the skeleton — the capability-filtered
// target select (source excluded), the copy-only pin (no mode radio, the
// confirm reads "Copy image"), the target-scoped preview's states, the
// stay-open report, and the error region. This adapter keeps the images
// domain: the manage-images capability, the copy preview query (the
// already-present fact is information in the warnings slot, never a
// block — the load is idempotent), the NDJSON transport, the report's
// loaded names, and the too-large hint. A close mid-run aborts the
// stream; the both-hosts invalidation lives in the stream hook, not
// here.
export function CopyImageDialog({
	host,
	image,
	onClose,
}: {
	host: Host;
	image: ImageSummary;
	onClose: () => void;
}) {
	const transfer = useTransferState(host, "manage-images");
	const copyStream = useImageCopyStream(host.id);
	const previewQuery = useImageCopyPreview(
		host.id,
		image.id,
		transfer.targetId || null,
	);
	// The preset's preview slice speaks collision; the image copy never
	// collides (a present target image is an idempotent load), so the
	// slice is structural only and the already-present fact narrates in
	// the warnings slot below.
	const preview: TransferPreviewSlice = {
		isPending: previewQuery.isPending,
		isError: previewQuery.isError,
		error: previewQuery.error,
		data: previewQuery.data ? { collision: false } : undefined,
	};

	// The terminal report of a completed copy: the shared skeleton
	// narrates the Copied header and byte count in place — the dialog
	// stays open on it, the stay-open-report pattern — and the loaded
	// names ride the children slot. A load without names renders no block
	// (the backend answers [], never null).
	const report = copyStream.result ? (
		<TransferReport
			header={{
				status: "copied",
				itemName: displayTags(image),
				targetName: transfer.targetName,
				bytes: copyStream.result.bytes,
			}}
		>
			{copyStream.result.names.length > 0 ? (
				<Box data-testid="copy-loaded-names">
					<Text>Loaded on {transfer.targetName} as:</Text>
					<VStack align="flex-start" mt={1} spacing={1}>
						{copyStream.result.names.map((name) => (
							<Text key={name} fontFamily="mono">
								{name}
							</Text>
						))}
					</VStack>
				</Box>
			) : null}
		</TransferReport>
	) : null;

	return (
		<TransferDialog
			host={host}
			transfer={transfer}
			preview={preview}
			pinnedMode="copy"
			noun="image"
			itemName={displayTags(image)}
			isPending={copyStream.isRunning}
			error={copyStream.error}
			renderHints={(error) =>
				error.code === IMAGE_TRANSFER_TOO_LARGE_CODE ? (
					<Text mt={2} color="orange.500" data-testid="copy-too-large-hint">
						The image&apos;s archive exceeds the server&apos;s transfer bound
						(IMAGE_TRANSFER_MAX_BYTES). Copy a smaller image or raise the bound
						on the server.
					</Text>
				) : null
			}
			report={report}
			warnings={
				previewQuery.data?.present ? (
					<Text color="blue.500" data-testid="copy-already-present">
						{transfer.targetName} already holds this image. The copy stays
						possible: the load skips present layers and re-applies the
						image&apos;s tags.
					</Text>
				) : null
			}
			onConfirm={() =>
				copyStream.run(image.id, { targetHostId: transfer.targetId })
			}
			onClose={onClose}
		>
			{copyStream.isRunning && copyStream.progress ? (
				<TransferProgress progress={copyStream.progress} />
			) : null}
		</TransferDialog>
	);
}
