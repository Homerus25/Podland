import { useToast } from "@chakra-ui/react";
import { volumeExportUrl } from "@features/volumes/services/volumesApi";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { useState } from "react";

// The volume archive export, owned once (spec #477): the list row and the
// volume detail page run the same ritual — a per-volume busy state, the
// volume's own export endpoint through the shared download helper, and an
// error toast naming the volume — so the two surfaces cannot drift. Export
// is a read, so the hook carries no capability gate: every caller gets it.
//
// Unlike the image download, the filename is explicit: the browser saves the
// archive under the volume's name. The busy state is the volume's name
// rather than a boolean, so a list row shows only its own in-flight export
// while the detail page — which only ever exports its one volume — reads the
// same field.
export function useVolumeExport(hostId: string) {
	const [exportingName, setExportingName] = useState<string | null>(null);
	const toast = useToast();

	const exportVolume = async (volumeName: string) => {
		setExportingName(volumeName);
		try {
			await downloadFile(
				volumeExportUrl(hostId, volumeName),
				`${volumeName}.tar`,
			);
		} catch (error) {
			toast({
				title: `Failed to export volume: ${volumeName}`,
				description: getApiError(error).message,
				status: "error",
				isClosable: true,
			});
		} finally {
			setExportingName(null);
		}
	};

	return { exportingName, exportVolume };
}
