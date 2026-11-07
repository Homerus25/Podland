import { Text } from "@chakra-ui/react";
import {
	type ImageCopyProgress,
	type ImageCopyStreamEvent,
	imageCopyStreamConfig,
} from "@features/images/hooks/useImageCopyStream";
import { useImageCopyPreviews } from "@features/images/hooks/useImages";
import { displayTags } from "@features/images/services/imageDisplayTags";
import type {
	ImageCopyResult,
	ImageSummary,
} from "@features/images/services/imagesApi";
import {
	MultiTransferDialog,
	type MultiTransferItem,
} from "@shared/components/MultiTransferDialog";
import { useTransferState } from "@shared/components/TransferDialog";
import {
	runMoveStream,
	transferErrorMessage,
} from "@shared/transfers/transferHooks";
import { transferProgressLabel } from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";
import { formatBytes } from "@shared/utils/format";
import { useQueryClient } from "@tanstack/react-query";

// The Images batch copy (spec 486, ticket #488): the shared multi-item
// dialog over the existing per-image copy endpoint. The adapter keeps the
// images domain — the manage-images capability the target list filters on,
// the per-image preview queries, the already-present note, the NDJSON
// transport (the endpoint always streams), and the envelope-to-outcome
// mapping. The both-hosts invalidation stays in the shared stream runner,
// where the single-item dialog already leaves it.
//
// The dialog is opened with a snapshot of the selected images, so the report
// keeps naming them after the list drops the successes from its selection.
export function CopyImagesDialog({
	host,
	images,
	onFinished,
	onClose,
}: {
	host: Host;
	images: ImageSummary[];
	onFinished: (succeededKeys: string[]) => void;
	onClose: () => void;
}) {
	const transfer = useTransferState(host, "manage-images");
	const previews = useImageCopyPreviews(
		host.id,
		images.map((image) => image.id),
		transfer.targetId || null,
	);
	const queryClient = useQueryClient();

	const items: MultiTransferItem<ImageSummary>[] = images.map(
		(image, index) => {
			const preview = previews[index];
			return {
				item: image,
				// The selection key is the image's id — the identity the copy
				// endpoint addresses, and the key the list's selection holds.
				key: image.id,
				// The same label the row renders.
				label: displayTags(image),
				// The preset's preview slice speaks collision; an image copy
				// never collides (a present target image is an idempotent
				// load), so the slice is structural only and the
				// already-present fact narrates in the warnings slot below.
				preview: {
					isPending: preview.isPending,
					isError: preview.isError,
					error: preview.error,
					data: preview.data ? { collision: false } : undefined,
				},
				// The same note the single-item dialog renders, under this
				// image's own name.
				warnings: preview.data?.present ? (
					<Text color="blue.500" data-testid="copy-already-present">
						{transfer.targetName} already holds this image. The copy stays
						possible: the load skips present layers and re-applies the
						image&apos;s tags.
					</Text>
				) : null,
			};
		},
	);

	return (
		<MultiTransferDialog
			host={host}
			transfer={transfer}
			noun="image"
			// An image copy is copy-only: no mode radio, and the title and
			// confirm speak Copy vocabulary only.
			pinnedMode="copy"
			items={items}
			runItem={async (image, context) => {
				try {
					// The result type is named explicitly: it appears only in the
					// runner's return type, so inference would fall back to the
					// structural both-hosts constraint and lose the envelope's
					// byte count.
					const result = await runMoveStream<
						ImageCopyStreamEvent,
						ImageCopyProgress | null,
						ImageCopyResult
					>(
						imageCopyStreamConfig(host.id),
						queryClient,
						image.id,
						{ targetHostId: context.targetId },
						{
							signal: context.signal,
							// The fold starts at null (no progress event yet), and
							// the report row keeps its "Running…" line until the
							// first one arrives.
							onProgress: (progress) => {
								if (progress)
									context.onProgress(transferProgressLabel(progress));
							},
						},
					);
					return {
						ok: true,
						detail: `Copied (${formatBytes(result.bytes)})`,
					};
				} catch (error) {
					// A terminal error event or a transport failure is this
					// item's failure, not the batch's: the report names it and
					// the rest of the run continues.
					return { ok: false, detail: transferErrorMessage(error) };
				}
			}}
			onFinished={onFinished}
			onClose={onClose}
		/>
	);
}
