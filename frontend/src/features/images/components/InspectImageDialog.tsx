import { displayTags } from "@features/images/components/imageDisplayTags";
import { useImageInspect } from "@features/images/hooks/useImages";
import type { ImageSummary } from "@features/images/services/imagesApi";
import { InspectDialog } from "@shared/components/InspectDialog";

// Thin adapter between the images feature and the shared inspect dialog:
// it owns the query (fired on mount, dead on unmount, gcTime 0 so every
// open refetches) and hands the dialog its structural slice; the dialog
// owns the whole render. The title is the image's tags, derived here like
// the other adapters derive theirs.
export function InspectImageDialog({
	machineId,
	image,
	onClose,
}: {
	machineId: string;
	image: ImageSummary;
	onClose: () => void;
}) {
	const query = useImageInspect(machineId, image.id);
	return (
		<InspectDialog title={displayTags(image)} query={query} onClose={onClose} />
	);
}
