import { Box, Button, Td, Text, Tr } from "@chakra-ui/react";
import { CopyImageDialog } from "@features/images/components/CopyImageDialog";
import { displayTags } from "@features/images/components/imageDisplayTags";
import { PullImageForm } from "@features/images/components/PullImageForm";
import {
	useDeleteImage,
	useImages,
	usePruneImages,
} from "@features/images/hooks/useImages";
import type { ImageSummary } from "@features/images/services/imagesApi";
import { can } from "@features/machines/permissions";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Machine } from "@shared/types/api";
import {
	formatBytes,
	formatRelativeDate,
	shortImageId,
} from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The delete adapter over the shared force-refusal preset: the module owns
// the confirm-to-force ritual (the in-use hint, the footer swap, the force
// flag keyed off the current error); this adapter keeps its domain only —
// the IMAGE_USED_BY_CONTAINERS refusal code, the hint copy, the labels,
// the testids, and the payload shape.
function DeleteImageModal({
	image,
	onClose,
	deleteImage,
}: {
	image: ImageSummary;
	onClose: () => void;
	deleteImage: ReturnType<typeof useDeleteImage>;
}) {
	return (
		<ForceConfirmDialog
			mutation={deleteImage}
			title="Delete image"
			onClose={onClose}
			refusalCode="IMAGE_USED_BY_CONTAINERS"
			hint="One or more containers use this image, so it cannot be deleted normally. Force delete removes the image even though containers still use it; those containers may stop working."
			hintTestId="image-in-use-warning"
			confirmLabel="Delete image"
			forceLabel="Force delete"
			forceTestId="force-delete-button"
			confirm={(force) => ({ imageId: image.id, force })}
			body={
				<Text>
					Delete {displayTags(image)} ({shortImageId(image.id)}) from this
					machine? This cannot be undone.
				</Text>
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
	const [pendingCopy, setPendingCopy] = useState<ImageSummary | null>(null);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const canManage = can(machine, "manage-images");
	const navigate = useNavigate();

	return (
		<>
			<ResourceList
				resource="image"
				isPending={isPending}
				error={error}
				items={images}
				columns={[
					{ label: "Tag", isName: true, sortBy: (img) => displayTags(img) },
					{ label: "Image ID", sortBy: (img) => shortImageId(img.id) },
					{ label: "Size", isNumeric: true, sortBy: (img) => img.size },
					{ label: "Created", sortBy: (img) => img.createdAt },
					{},
				]}
				renderRow={(image) => (
					// The row is clickable: it navigates to the image's detail
					// page, and the delete button stops propagation so a button
					// click never also fires the row action.
					<Tr
						key={image.id}
						data-testid="image-row"
						cursor="pointer"
						onClick={() => navigate(detailPath(machineId, "images", image.id))}
					>
						<Td fontFamily="mono">{displayTags(image)}</Td>
						<Td fontFamily="mono" color="gray.500">
							{shortImageId(image.id)}
						</Td>
						<Td isNumeric>{formatBytes(image.size)}</Td>
						<Td>{formatRelativeDate(image.createdAt)}</Td>
						<Td>
							{canManage ? (
								<Button
									size="xs"
									colorScheme="blue"
									variant="outline"
									aria-label={`Copy ${displayTags(image)} to another machine`}
									onClick={(event) => {
										event.stopPropagation();
										setPendingCopy(image);
									}}
									data-testid="copy-image-button"
								>
									Copy to machine
								</Button>
							) : null}
							<Button
								size="xs"
								colorScheme="red"
								variant="outline"
								aria-label={`Delete ${displayTags(image)}`}
								onClick={(event) => {
									event.stopPropagation();
									setPendingDelete(image);
								}}
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
						Prune
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
			{pendingCopy ? (
				<CopyImageDialog
					machine={machine}
					image={pendingCopy}
					onClose={() => setPendingCopy(null)}
				/>
			) : null}
		</>
	);
}
