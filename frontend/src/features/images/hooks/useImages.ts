import {
	bulkRemoveImages,
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
import type { BulkRemoveInput } from "@shared/types/api";

export function useImages(hostId: string) {
	return useAlwaysRefetchListQuery(imagesQueryKey(hostId), () =>
		listImages(hostId),
	);
}

export function useImage(hostId: string, imageId: string) {
	return useOneShotQuery(imageQueryKey(hostId, imageId), () =>
		getImage(hostId, imageId),
	);
}

export function useImageInspect(
	hostId: string,
	imageId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		imageInspectQueryKey(hostId, imageId),
		() => inspectImage(hostId, imageId),
		options,
	);
}

export function useDeleteImage(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"images",
		({ imageId, force }: { imageId: string; force?: boolean }) =>
			deleteImage(hostId, imageId, force ?? false),
	);
}

// A bulk remove changes the host's image set, so the list refetches on
// success — including after a partial run, where the successes are gone and
// the refused images stay. The report is the mutation's own data, so the
// dialog reads it straight off the result.
export function useBulkRemoveImages(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"images",
		(input: BulkRemoveInput) => bulkRemoveImages(hostId, input),
	);
}

export function usePruneImages(hostId: string) {
	return useListInvalidatingMutation(hostId, "images", () =>
		pruneImages(hostId),
	);
}

// The copy dialog's fact payload, keyed by the chosen target so a target
// change refetches. The ritual — the null-target guard, the empty-string
// placeholder key, the enabled gate — is the shared transfer-hooks
// module's (issue 273); this adapter supplies the key constructor and
// fetcher.
export function useImageCopyPreview(
	hostId: string,
	imageId: string,
	targetHostId: string | null,
) {
	return useMovePreview(
		(target) => imageCopyPreviewQueryKey(hostId, imageId, target),
		(target) => getImageCopyPreview(hostId, imageId, target),
		targetHostId,
	);
}
