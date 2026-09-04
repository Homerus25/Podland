import {
	deleteImage,
	listImages,
	pruneImages,
} from "@features/images/services/imagesApi";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

function imagesQueryKey(machineId: string) {
	return ["machines", machineId, "images"] as const;
}

export function useImages(machineId: string) {
	return useQuery({
		queryKey: imagesQueryKey(machineId),
		queryFn: () => listImages(machineId),
	});
}

export function useDeleteImage(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({ imageId, force }: { imageId: string; force?: boolean }) =>
			deleteImage(machineId, imageId, force ?? false),
		onSuccess: () => {
			queryClient.invalidateQueries({
				queryKey: imagesQueryKey(machineId),
			});
		},
	});
}

export function usePruneImages(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: () => pruneImages(machineId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: imagesQueryKey(machineId) });
		},
	});
}
