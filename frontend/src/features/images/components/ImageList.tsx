import { Box, Button, Td, Text, Tr } from "@chakra-ui/react";
import { PullImageForm } from "@features/images/components/PullImageForm";
import {
	useDeleteImage,
	useImages,
	usePruneImages,
} from "@features/images/hooks/useImages";
import type { ImageSummary } from "@features/images/services/imagesApi";
import { can } from "@features/machines/permissions";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import {
	formatBytes,
	formatRelativeDate,
	shortImageId,
} from "@shared/utils/format";
import { useState } from "react";

function imageTags(image: ImageSummary): string[] {
	const tags = (image.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
	return tags.length > 0 ? tags : ["<none>"];
}

function displayTags(image: ImageSummary): string {
	return imageTags(image).join(", ");
}

const USED_BY_CONTAINERS_CODE = "IMAGE_USED_BY_CONTAINERS";

function isUsedByContainers(error: unknown): boolean {
	return getApiError(error).code === USED_BY_CONTAINERS_CODE;
}

// The delete adapter over ConfirmDialog. This is where both replacement
// seams prove out: on the IMAGE_USED_BY_CONTAINERS error the footer's
// confirm button swaps to the outline force button (renderConfirm), and the
// in-use hint replaces the whole error region (renderError); any other
// error renders through the shared ErrorDisplay inside the same seam. The
// force flag keys off the current error code exactly as before: the first
// confirm requests a plain delete, and only after the in-use rejection does
// the force request carry force=true.
function DeleteImageModal({
	image,
	onClose,
	deleteImage,
}: {
	image: ImageSummary;
	onClose: () => void;
	deleteImage: ReturnType<typeof useDeleteImage>;
}) {
	const inUse = isUsedByContainers(deleteImage.error);

	return (
		<ConfirmDialog
			mutation={deleteImage}
			title="Delete image"
			confirm={() => ({ imageId: image.id, force: inUse })}
			onClose={onClose}
			body={
				<Text>
					Delete {displayTags(image)} ({shortImageId(image.id)}) from this
					machine? This cannot be undone.
				</Text>
			}
			renderError={(apiError) =>
				apiError.code === USED_BY_CONTAINERS_CODE ? (
					<Text mt={2} color="orange.500" data-testid="image-in-use-warning">
						One or more containers use this image, so it cannot be deleted
						normally. Force delete removes the image even though containers
						still use it; those containers may stop working.
					</Text>
				) : (
					<ErrorDisplay error={apiError} />
				)
			}
			renderConfirm={({ error, isPending, onConfirm }) =>
				isUsedByContainers(error) ? (
					<Button
						colorScheme="red"
						variant="outline"
						onClick={onConfirm}
						isLoading={isPending}
						data-testid="force-delete-button"
					>
						Force delete
					</Button>
				) : (
					<Button
						colorScheme="red"
						onClick={onConfirm}
						isLoading={isPending}
						data-testid="confirm-delete-button"
					>
						Delete image
					</Button>
				)
			}
		/>
	);
}

// The prune adapter over PruneReportDialog. The module owns the reset
// lifecycle, the confirm/report footer swap, the report table, and the
// noun-derived copy; this adapter hands over the mutation result plus the
// entry accessors that make an image entry a report row.
function PruneImagesModal({
	onClose,
	pruneImages,
}: {
	onClose: () => void;
	pruneImages: ReturnType<typeof usePruneImages>;
}) {
	return (
		<PruneReportDialog
			mutation={pruneImages}
			resource="image"
			entryKey={(entry) => entry.id}
			renderLabel={(entry) => shortImageId(entry.id)}
			onClose={onClose}
		/>
	);
}

// The Images tab body. The toolbar/triad/table machinery, the plural testids,
// and the hint swap are ResourceList's; this adapter owns only the columns,
// the row, and the pull/prune/delete feature code. The pull form is a write,
// so it rides the gated toolbar: a flex box first keeps it left-aligned with
// the prune button right-aligned, and below the write-gated manage capability
// the module swaps the whole toolbar for the canonical read hint.
export function ImageList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: images, isPending, error } = useImages(machineId);
	const deleteImage = useDeleteImage(machineId);
	const pruneImages = usePruneImages(machineId);
	const [pendingDelete, setPendingDelete] = useState<ImageSummary | null>(null);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const canManage = can(machine, "manage-images");

	return (
		<>
			<ResourceList
				resource="image"
				isPending={isPending}
				error={error}
				items={images}
				columns={[
					{ label: "Tag", sortBy: (img) => displayTags(img) },
					{ label: "Image ID", sortBy: (img) => shortImageId(img.id) },
					{ label: "Size", isNumeric: true, sortBy: (img) => img.size },
					{ label: "Created", sortBy: (img) => img.createdAt },
					{},
				]}
				renderRow={(image) => (
					<Tr key={image.id} data-testid="image-row">
						<Td fontFamily="mono">{displayTags(image)}</Td>
						<Td fontFamily="mono" color="gray.500">
							{shortImageId(image.id)}
						</Td>
						<Td isNumeric>{formatBytes(image.size)}</Td>
						<Td>{formatRelativeDate(image.createdAt)}</Td>
						<Td>
							<Button
								size="xs"
								colorScheme="red"
								variant="outline"
								aria-label={`Delete ${displayTags(image)}`}
								onClick={() => setPendingDelete(image)}
							>
								Delete
							</Button>
						</Td>
					</Tr>
				)}
				toolbar={[
					<Box key="pull-image-form" flex={1}>
						<PullImageForm machineId={machineId} />
					</Box>,
					<Button
						key="prune-images-button"
						size="sm"
						colorScheme="red"
						variant="outline"
						alignSelf="flex-start"
						onClick={() => setIsPruneOpen(true)}
						data-testid="prune-images-button"
					>
						Delete unused images
					</Button>,
				]}
				canManage={canManage}
				emptyMessage="No images on this machine yet. Pull one to get started."
			/>
			{isPruneOpen ? (
				<PruneImagesModal
					onClose={() => setIsPruneOpen(false)}
					pruneImages={pruneImages}
				/>
			) : null}
			{pendingDelete ? (
				<DeleteImageModal
					image={pendingDelete}
					onClose={() => setPendingDelete(null)}
					deleteImage={deleteImage}
				/>
			) : null}
		</>
	);
}
