import { Button, Table, Tbody, Text } from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { useHost } from "@features/hosts/hooks/useHosts";
import { CopyImageDialog } from "@features/images/components/CopyImageDialog";
import { IMAGE_IN_USE_REFUSAL } from "@features/images/components/imageRemoval";
import { useImageDownload } from "@features/images/hooks/useImageDownload";
import {
	useDeleteImage,
	useImage,
	useImageInspect,
} from "@features/images/hooks/useImages";
import {
	displayTags,
	imageDisplayName,
} from "@features/images/services/imageDisplayTags";
import type { ImageDetail } from "@features/images/services/imagesApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	headerActionGroup,
	isNotFound,
	KeyValueTable,
	removalAction,
} from "@shared/components/detailKit";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { hostTabPath } from "@shared/routes/detailRoute";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

function ImageOverview({ image }: { image: ImageDetail }) {
	return (
		<Table size="sm">
			<Tbody>
				<DetailRow label="Name">
					<Text fontFamily="mono">{displayTags(image)}</Text>
				</DetailRow>
				<DetailRow label="Id">
					<Text fontFamily="mono" wordBreak="break-all">
						{image.id}
					</Text>
				</DetailRow>
				<DetailRow label="Created">
					{formatRelativeDate(image.createdAt)}
				</DetailRow>
				<DetailRow label="User">{image.user ? image.user : "—"}</DetailRow>
				<DetailRow label="Env">
					<KeyValueTable testId="image-detail-env" values={image.env} />
				</DetailRow>
				<DetailRow label="Entrypoint">
					<Text fontFamily="mono">{image.entrypoint?.join(" ") || "—"}</Text>
				</DetailRow>
				<DetailRow label="Cmd">
					<Text fontFamily="mono">{image.cmd?.join(" ") || "—"}</Text>
				</DetailRow>
				<DetailRow label="Labels">
					<KeyValueTable testId="image-detail-labels" values={image.labels} />
				</DetailRow>
				<DetailRow label="Size">{formatBytes(image.size)}</DetailRow>
			</Tbody>
		</Table>
	);
}

// The Remove action's confirm (spec 461, ticket #465): the shared
// force-refusal preset, so the first confirm sends the plain delete and
// only the IMAGE_USED_BY_CONTAINERS refusal swaps in the in-use hint and
// the force button whose retry carries force=true. The adapter keeps the
// domain vocabulary only — the refusal words come from the imageRemoval
// module the list's modal reads too, and the success side effect (navigate
// back to the list tab) arrives through the preset's onSuccess seam.
function RemoveImageDialog({
	image,
	onClose,
	deleteImage,
	onSuccess,
}: {
	image: ImageDetail;
	onClose: () => void;
	deleteImage: ReturnType<typeof useDeleteImage>;
	onSuccess: () => void;
}) {
	return (
		<ForceConfirmDialog
			mutation={deleteImage}
			title="Remove image"
			onClose={onClose}
			refusalCode={IMAGE_IN_USE_REFUSAL.code}
			hint={IMAGE_IN_USE_REFUSAL.hint}
			hintTestId={IMAGE_IN_USE_REFUSAL.hintTestId}
			confirmLabel="Remove image"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ imageId: image.id, force })}
			onSuccess={onSuccess}
			body={
				<Text>
					Remove {imageDisplayName(image)} from this host? This cannot be
					undone.
				</Text>
			}
		/>
	);
}

// The image detail page (spec 301): the adapter over the shared DetailPage
// layout. It owns the overview query (GET .../images/{imageId}) and the
// lazy inspect query (enabled on first reveal), and hands the layout their
// structural slices; the layout owns the whole render.
export function ImageDetailPage() {
	const { hostId, imageId } = useParams<{
		hostId: string;
		imageId: string;
	}>();
	const navigate = useNavigate();
	const [revealed, setRevealed] = useState(false);
	// The remove confirm (spec 461, ticket #465): rendered conditionally, so
	// mount is open and unmount is close.
	const [removeOpen, setRemoveOpen] = useState(false);
	// The copy dialog's open state (spec 477, ticket #481): rendered
	// conditionally, so mount is open and unmount is close — the remove
	// confirm's pattern.
	const [copyOpen, setCopyOpen] = useState(false);
	const { data: image, isPending, error } = useImage(hostId!, imageId!);
	const inspectQuery = useImageInspect(hostId!, imageId!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "manage-images");
	// The copy dialog needs the host itself (its transfer state filters the
	// targets by the caller's capability); the gate's hook rides the same
	// host query, so this adds no request.
	const host = useHost(hostId!);
	const deleteImage = useDeleteImage(hostId!);
	const notFound = isNotFound(error);

	// The Remove action rides the detail-page capability gate (the kit owns
	// the cascade): allowed renders the button, denied the shared write hint,
	// pending neither, and a not-found overview hides it outright.
	const removeAction = removalAction({
		gate: gate,
		notFound,
		scope: "images",
		resource: "image",
		testId: "remove-image-button",
		onClick: () => setRemoveOpen(true),
	});

	// The archive download is read-level (the volume Export button's
	// ruling), so it renders for every caller. The ritual — the busy state,
	// the export endpoint, the error toast — is the shared hook's, so the
	// list row's Download button runs the identical one.
	const { downloadingId, download } = useImageDownload(hostId!);

	// The header action row (spec #477), composed through the detail kit's
	// one owner: the row's actions beside Remove. Download reads at read
	// level, so it is ungated and renders for every caller; Copy to host
	// rides the manage-images gate and passes no hint, because Remove's
	// cascade already renders the page's write hint for the same capability
	// (the one-hint rule). Every control self-gates on the image DTO, since
	// the header renders while the overview query is still in flight.
	const headerActions = headerActionGroup({
		actions: [
			{
				control: image ? (
					<Button
						size="sm"
						variant="outline"
						aria-label={`Download ${displayTags(image)}`}
						isLoading={downloadingId === image.id}
						onClick={() => download(image)}
						data-testid="download-image-button"
					>
						Download
					</Button>
				) : null,
			},
			{
				gate,
				control: image ? (
					<Button
						size="sm"
						colorScheme="blue"
						variant="outline"
						aria-label={`Copy ${displayTags(image)} to another host`}
						onClick={() => setCopyOpen(true)}
						data-testid="copy-image-button"
					>
						Copy to host
					</Button>
				) : null,
			},
		],
		remove: removeAction,
	});

	return (
		<>
			<DetailPage
				title={image ? displayTags(image) : imageId!}
				backTo="images"
				isPending={isPending}
				error={error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={headerActions}
			>
				{image ? <ImageOverview image={image} /> : null}
			</DetailPage>
			{removeOpen && image ? (
				<RemoveImageDialog
					image={image}
					onClose={() => setRemoveOpen(false)}
					deleteImage={deleteImage}
					// The removed image's page would 404 on the next refetch, so
					// the page leaves for the list tab; the mutation's own
					// invalidation refetches that list.
					onSuccess={() => navigate(hostTabPath(hostId!, "images"))}
				/>
			) : null}
			{/* The shared copy dialog, the same one the list row opens, with
			    this page's image fixed. It needs the full host object, so it
			    waits for the host query the gate already rides. */}
			{copyOpen && image && host.data ? (
				<CopyImageDialog
					host={host.data}
					image={image}
					onClose={() => setCopyOpen(false)}
				/>
			) : null}
		</>
	);
}
