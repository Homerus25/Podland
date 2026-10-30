import {
	deleteImage,
	getImage,
	getImageCopyPreview,
	inspectImage,
	listImages,
	pruneImages,
} from "@features/images/services/imagesApi";
import {
	imageCopyPreviewQueryKey,
	imageInspectQueryKey,
	imageQueryKey,
	imagesQueryKey,
} from "@shared/queries/queryKeys";
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useMovePreview } from "@shared/transfers/transferHooks";

export function useImages(machineId: string) {
	return useAlwaysRefetchListQuery(imagesQueryKey(machineId), () =>
		listImages(machineId),
	);
}

export function useImage(machineId: string, imageId: string) {
	return useOneShotQuery(imageQueryKey(machineId, imageId), () =>
		getImage(machineId, imageId),
	);
}

export function useImageInspect(
	machineId: string,
	imageId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		imageInspectQueryKey(machineId, imageId),
		() => inspectImage(machineId, imageId),
		options,
	);
}

export function useDeleteImage(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"images",
		({ imageId, force }: { imageId: string; force?: boolean }) =>
			deleteImage(machineId, imageId, force ?? false),
	);
}

export function usePruneImages(machineId: string) {
	return useListInvalidatingMutation(machineId, "images", () =>
		pruneImages(machineId),
	);
}

// The copy dialog's fact payload, keyed by the chosen target so a target
// change refetches. The ritual — the null-target guard, the empty-string
// placeholder key, the enabled gate — is the shared transfer-hooks
// module's (issue 273); this adapter supplies the key constructor and
// fetcher.
export function useImageCopyPreview(
	machineId: string,
	imageId: string,
	targetMachineId: string | null,
) {
	return useMovePreview(
		(target) => imageCopyPreviewQueryKey(machineId, imageId, target),
		(target) => getImageCopyPreview(machineId, imageId, target),
		targetMachineId,
	);
}
