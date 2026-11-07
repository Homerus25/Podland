import { Box, Button, Td, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CopyImagesDialog } from "@features/images/components/CopyImagesDialog";
import { PullImageForm } from "@features/images/components/PullImageForm";
import { useImageDownload } from "@features/images/hooks/useImageDownload";
import {
	useBulkRemoveImages,
	useImages,
	usePruneImages,
} from "@features/images/hooks/useImages";
import {
	displayTags,
	imageDisplayName,
} from "@features/images/services/imageDisplayTags";
import type { ImageSummary } from "@features/images/services/imagesApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";

import {
	ResourceList,
	type ResourceListSelectionSnapshot,
	snapshotSelection,
} from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import {
	formatBytes,
	formatRelativeDate,
	shortImageId,
} from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

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
// the row, and the pull/prune/bulk-remove feature code. The pull form is a
// write, so it rides the gated toolbar: a flex box first keeps it
// left-aligned with the prune button right-aligned, and below the write-gated
// manage capability the module swaps the whole toolbar for the canonical read
// hint.
//
// Removal has no per-row button (spec 461, ticket #470): a single image is
// removed from its detail page, several from the toolbar's "Remove selected".
// The cross-host copy follows the same shape (spec 486, ticket #488): the row
// button is gone, a single image is copied from its detail page, and several
// from the toolbar's "Copy selected to host" — one dialog for the whole
// selection, one target chosen once, N sequential per-item copy runs.
export function ImageList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: images, isPending, error } = useImages(hostId);
	const pruneImages = usePruneImages(hostId);
	const bulkRemoveImages = useBulkRemoveImages(hostId);
	// The bulk-remove dialog's snapshot: the selected images plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		images: ImageSummary[];
		clear: () => void;
	} | null>(null);
	// The batch copy's snapshot, the same shape as the bulk remove's: the
	// selected images plus the selection's partial reset. A copy clears only
	// the images that succeeded, so the failures stay selected for the retry.
	const [copySelectedTarget, setCopySelectedTarget] =
		useState<ResourceListSelectionSnapshot<ImageSummary> | null>(null);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const canManage = can(host, "manage-images");
	const navigate = useNavigate();
	// The archive download is read-level (the volume Export button's
	// ruling): the button renders for every grantee, with a per-row busy
	// state while its download streams. The ritual is the shared hook's, so
	// the detail page's Download button runs the identical one.
	const { downloadingId, download } = useImageDownload(hostId);

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
					// page, and every action button stops propagation so a
					// button click never also fires the row action.
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
									download(image);
								}}
							>
								Download
							</Button>
						</Td>
					</Tr>
				)}
				toolbar={
					<>
						<Box flex={1}>
							<PullImageForm hostId={hostId} />
						</Box>
						<Button
							size="sm"
							colorScheme="red"
							variant="outline"
							alignSelf="flex-start"
							onClick={() => setIsPruneOpen(true)}
							data-testid="prune-images-button"
						>
							Prune
						</Button>
					</>
				}
				canManage={canManage}
				selection={{ getKey: (image) => image.id }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						images: selection.selectedItems,
						clear: selection.clear,
					})
				}
				// The batch copy rides manage-images — the capability the row
				// button it replaces used, and the one the copy endpoint
				// enforces — which is also the list's `canManage`, so the
				// module's own write gate is the whole gate: no second
				// predicate to keep in sync.
				selectionActions={(selection) => (
					<Button
						size="sm"
						variant="outline"
						isDisabled={selection.selectedKeys.length === 0}
						onClick={() => setCopySelectedTarget(snapshotSelection(selection))}
						data-testid="copy-selected-images-button"
					>
						Copy selected to host
					</Button>
				)}
				emptyMessage="No images on this host yet. Pull one to get started."
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.images}
					noun="image"
					itemKey={(image) => image.id}
					itemLabel={(image) => imageDisplayName(image)}
					mutation={bulkRemoveImages}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
			{isPruneOpen ? (
				<PruneImagesModal
					onClose={() => setIsPruneOpen(false)}
					pruneImages={pruneImages}
				/>
			) : null}
			{copySelectedTarget ? (
				<CopyImagesDialog
					host={host}
					images={copySelectedTarget.items}
					// Only the succeeded images leave the selection.
					onFinished={copySelectedTarget.clearKeys}
					onClose={() => setCopySelectedTarget(null)}
				/>
			) : null}
		</>
	);
}
