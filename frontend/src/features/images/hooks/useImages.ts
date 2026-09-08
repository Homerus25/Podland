import {
	deleteImage,
	listImages,
	pruneImages,
} from "@features/images/services/imagesApi";
import { imagesQueryKey } from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useImages(machineId: string) {
	return useQuery({
		queryKey: imagesQueryKey(machineId),
		queryFn: () => listImages(machineId),
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
