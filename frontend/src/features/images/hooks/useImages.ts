import {
	deleteImage,
	inspectImage,
	listImages,
	pruneImages,
} from "@features/images/services/imagesApi";
import {
	imageInspectQueryKey,
	imagesQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useImages(machineId: string) {
	return useQuery({
		queryKey: imagesQueryKey(machineId),
		queryFn: () => listImages(machineId),
	});
}

// The raw inspect report behind the inspect dialog. gcTime: 0 drops the
// cache entry the moment the dialog unmounts, so every open refetches the
// current report.
export function useImageInspect(machineId: string, imageId: string) {
	return useQuery({
		queryKey: imageInspectQueryKey(machineId, imageId),
		queryFn: () => inspectImage(machineId, imageId),
		gcTime: 0,
	});
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
