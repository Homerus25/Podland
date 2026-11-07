import {
	bulkRemoveVolumes,
	createVolume,
	deleteVolume,
	getVolumeMovePreview,
	listVolumes,
	populateVolume,
	pruneVolumes,
	type VolumeCreateInput,
} from "@features/volumes/services/volumesApi";
import {
	volumeMovePreviewQueryKey,
	volumesQueryKey,
} from "@shared/queries/queryKeys";
import { useAlwaysRefetchListQuery } from "@shared/queries/useFreshQuery";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import {
	useMovePreview,
	useMovePreviews,
} from "@shared/transfers/transferHooks";
import type { BulkRemoveInput } from "@shared/types/api";

export function useVolumes(hostId: string) {
	return useAlwaysRefetchListQuery(volumesQueryKey(hostId), () =>
		listVolumes(hostId),
	);
}

// A create extends the host's volume list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateVolume(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"volumes",
		(input: VolumeCreateInput) => createVolume(hostId, input),
	);
}

// A delete removes one volume from the host's list, so the list
// refetches on success; no optimistic updates.
export function useDeleteVolume(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"volumes",
		({ volumeName, force }: { volumeName: string; force: boolean }) =>
			deleteVolume(hostId, volumeName, force),
	);
}

// A prune changes the host's volume set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneVolumes(hostId: string) {
	return useListInvalidatingMutation(hostId, "volumes", () =>
		pruneVolumes(hostId),
	);
}

// A bulk remove changes the host's volume set, so the list refetches on
// success — including after a partial run, where the successes are gone and
// the refused volumes stay. The report is the mutation's own data, so the
// dialog reads it straight off the result.
export function useBulkRemoveVolumes(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"volumes",
		(input: BulkRemoveInput) => bulkRemoveVolumes(hostId, input),
	);
}

// Populate changes a volume's contents (and reported size), so the list
// refetches on success; no optimistic updates.
export function usePopulateVolume(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"volumes",
		(input: {
			volumeName: string;
			form: FormData;
			onProgress?: (fraction: number) => void;
		}) =>
			populateVolume(hostId, input.volumeName, input.form, input.onProgress),
	);
}

// The transfer dialog's fact payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useVolumeMovePreview(
	hostId: string,
	volumeName: string,
	targetHostId: string | null,
) {
	return useMovePreview(
		(target) => volumeMovePreviewQueryKey(hostId, volumeName, target),
		(target) => getVolumeMovePreview(hostId, volumeName, target),
		targetHostId,
	);
}

// The batch transfer's per-volume previews (spec 486, ticket #490): one
// target-scoped query per selected volume, over the same key constructor and
// fetcher the single-item hook uses, so the two surfaces can never ask the
// backend different questions. The ritual is the shared transfer-hooks
// module's (useMovePreviews); this adapter supplies the key constructor and
// fetcher.
export function useVolumeMovePreviews(
	hostId: string,
	volumeNames: string[],
	targetHostId: string | null,
) {
	return useMovePreviews(
		(volumeName, target) =>
			volumeMovePreviewQueryKey(hostId, volumeName, target),
		(volumeName, target) => getVolumeMovePreview(hostId, volumeName, target),
		volumeNames,
		targetHostId,
	);
}
