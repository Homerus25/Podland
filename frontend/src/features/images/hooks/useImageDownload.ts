import { useToast } from "@chakra-ui/react";
import { imageDisplayName } from "@features/images/services/imageDisplayTags";
import { imageExportUrl } from "@features/images/services/imagesApi";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { useState } from "react";

// The image archive download, owned once (spec #477): the list row and the
// image detail page run the same ritual — a per-image busy state, the export
// endpoint through the shared download helper, and an error toast naming the
// image through the feature's one naming composite — so the two surfaces
// cannot drift. The archive download is read-level (the volume Export
// button's ruling), so the hook carries no capability gate: every caller
// gets it.
//
// No filename is passed: the server names the archive in its
// Content-Disposition and the shared helper saves under that name. The busy
// state is the image's id rather than a boolean, so a list row shows only
// its own in-flight download while the detail page — which only ever
// downloads its one image — reads the same field.
export function useImageDownload(hostId: string) {
	const [downloadingId, setDownloadingId] = useState<string | null>(null);
	const toast = useToast();

	const download = async (image: { id: string; repoTags: string[] | null }) => {
		setDownloadingId(image.id);
		try {
			await downloadFile(imageExportUrl(hostId, image.id));
		} catch (error) {
			toast({
				title: `Failed to download image: ${imageDisplayName(image)}`,
				description: getApiError(error).message,
				status: "error",
				isClosable: true,
			});
		} finally {
			setDownloadingId(null);
		}
	};

	return { downloadingId, download };
}
