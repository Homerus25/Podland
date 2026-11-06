import { Table, Tbody, Text } from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import {
	displayTags,
	imageDisplayName,
} from "@features/images/components/imageDisplayTags";
import { IMAGE_IN_USE_REFUSAL } from "@features/images/components/imageRemoval";
import {
	useDeleteImage,
	useImage,
	useImageInspect,
} from "@features/images/hooks/useImages";
import type { ImageDetail } from "@features/images/services/imagesApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
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
	const { data: image, isPending, error } = useImage(hostId!, imageId!);
	const inspectQuery = useImageInspect(hostId!, imageId!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "manage-images");
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
				headerActions={removeAction}
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
		</>
	);
}
