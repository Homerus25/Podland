import { Box, Button, Td, Text, Tr, useToast } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CopyImageDialog } from "@features/images/components/CopyImageDialog";
import { displayTags } from "@features/images/components/imageDisplayTags";
import { PullImageForm } from "@features/images/components/PullImageForm";
import {
	useDeleteImage,
	useImages,
	usePruneImages,
} from "@features/images/hooks/useImages";
import {
	type ImageSummary,
	imageExportUrl,
} from "@features/images/services/imagesApi";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
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
					Delete {displayTags(image)} ({shortImageId(image.id)}) from this host?
					This cannot be undone.
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

// The Images tab body. The toolbar/triad/table plumbing, the plural testids,
// and the hint swap are ResourceList's; this adapter owns only the columns,
// the row, and the pull/prune/delete feature code. The pull form is a write,
// so it rides the gated toolbar: a flex box first keeps it left-aligned with
// the prune button right-aligned, and below the write-gated manage capability
// the module swaps the whole toolbar for the canonical read hint.
export function ImageList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: images, isPending, error } = useImages(hostId);
	const deleteImage = useDeleteImage(hostId);
	const pruneImages = usePruneImages(hostId);
	const [pendingDelete, setPendingDelete] = useState<ImageSummary | null>(null);
	const [pendingCopy, setPendingCopy] = useState<ImageSummary | null>(null);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [downloadingId, setDownloadingId] = useState<string | null>(null);
	const canManage = can(host, "manage-images");
	const navigate = useNavigate();
	const toast = useToast();

	// The archive download is read-level (the volume Export button's
	// ruling): the button renders for every grantee, with a per-row busy
	// state while its download streams. No filename is passed — the
	// server names the archive in its Content-Disposition and the shared
	// download helper saves under that name.
	const handleDownload = async (image: ImageSummary) => {
		setDownloadingId(image.id);
		try {
			await downloadFile(imageExportUrl(hostId, image.id));
		} catch (error) {
			// The delete modal's naming composite: tags alone render
			// "<none>" for an untagged image, so the short ID names it.
			toast({
				title: `Failed to download image: ${displayTags(image)} (${shortImageId(image.id)})`,
				description: getApiError(error).message,
				status: "error",
				isClosable: true,
			});
		} finally {
			setDownloadingId(null);
		}
	};

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
						onClick={() => navigate(detailPath(hostId, "images", image.id))}
					>
						<Td fontFamily="mono">{displayTags(image)}</Td>
						<Td fontFamily="mono" color="gray.500">
							{shortImageId(image.id)}
						</Td>
						<Td isNumeric>{formatBytes(image.size)}</Td>
						<Td>{formatRelativeDate(image.createdAt)}</Td>
						<Td>
							<Button
								size="xs"
								variant="outline"
								aria-label={`Download ${displayTags(image)}`}
								isLoading={downloadingId === image.id}
								onClick={(event) => {
									event.stopPropagation();
									handleDownload(image);
								}}
							>
								Download
							</Button>
							{canManage ? (
								<Button
									size="xs"
									colorScheme="blue"
									variant="outline"
									aria-label={`Copy ${displayTags(image)} to another host`}
									onClick={(event) => {
										event.stopPropagation();
										setPendingCopy(image);
									}}
									data-testid="copy-image-button"
								>
									Copy to host
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
						<PullImageForm hostId={hostId} />
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
				emptyMessage="No images on this host yet. Pull one to get started."
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
					host={host}
					image={pendingCopy}
					onClose={() => setPendingCopy(null)}
				/>
			) : null}
		</>
	);
}
