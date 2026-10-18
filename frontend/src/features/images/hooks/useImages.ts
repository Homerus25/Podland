import {
	deleteImage,
	getImage,
	inspectImage,
	listImages,
	pruneImages,
} from "@features/images/services/imagesApi";
import {
	imageInspectQueryKey,
	imageQueryKey,
	imagesQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useImages(machineId: string) {
	return useQuery({
		queryKey: imagesQueryKey(machineId),
		queryFn: () => listImages(machineId),
		// Lazy tabs remount the panel on every activation; refetching on
		// mount makes each activation issue a fresh request, even inside the
		// app-wide five-minute stale window.
		refetchOnMount: "always",
	});
}

// The image detail view (GET .../images/{imageId}, spec 301): the detail
// page's overview query. gcTime: 0 drops the cache entry the moment the
// page unmounts, so every page visit refetches the current detail.
export function useImage(machineId: string, imageId: string) {
	return useQuery({
		queryKey: imageQueryKey(machineId, imageId),
		queryFn: () => getImage(machineId, imageId),
		gcTime: 0,
	});
}

// The raw inspect report behind the inspect dialog. gcTime: 0 drops the
// cache entry the moment the dialog unmounts, so every open refetches the
// current report. The detail page passes enabled: revealed so the report
// fetches lazily on the first "Show inspect JSON" click, not on page mount.
export function useImageInspect(
	machineId: string,
	imageId: string,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey: imageInspectQueryKey(machineId, imageId),
		queryFn: () => inspectImage(machineId, imageId),
		enabled: options?.enabled ?? true,
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
