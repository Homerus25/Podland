import {
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
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useMovePreview } from "@shared/transfers/transferHooks";
import { useQuery } from "@tanstack/react-query";

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: volumesQueryKey(machineId),
		queryFn: () => listVolumes(machineId),
		// Lazy tabs remount the panel on every activation; refetching on
		// mount makes each activation issue a fresh request, even inside the
		// app-wide five-minute stale window.
		refetchOnMount: "always",
	});
}

// A create extends the machine's volume list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		(input: VolumeCreateInput) => createVolume(machineId, input),
	);
}

// A delete removes one volume from the machine's list, so the list
// refetches on success; no optimistic updates.
export function useDeleteVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		({ volumeName, force }: { volumeName: string; force: boolean }) =>
			deleteVolume(machineId, volumeName, force),
	);
}

// A prune changes the machine's volume set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneVolumes(machineId: string) {
	return useListInvalidatingMutation(machineId, "volumes", () =>
		pruneVolumes(machineId),
	);
}

// Populate changes a volume's contents (and reported size), so the list
// refetches on success; no optimistic updates.
export function usePopulateVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		(input: {
			volumeName: string;
			form: FormData;
			onProgress?: (fraction: number) => void;
		}) =>
			populateVolume(machineId, input.volumeName, input.form, input.onProgress),
	);
}

// The transfer dialog's fact payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useVolumeMovePreview(
	machineId: string,
	volumeName: string,
	targetMachineId: string | null,
) {
	return useMovePreview(
		(target) => volumeMovePreviewQueryKey(machineId, volumeName, target),
		(target) => getVolumeMovePreview(machineId, volumeName, target),
		targetMachineId,
	);
}
