import {
	createVolume,
	deleteVolume,
	listVolumes,
	populateVolume,
	pruneVolumes,
	type VolumeCreateInput,
} from "@features/volumes/services/volumesApi";
import { volumesQueryKey } from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: volumesQueryKey(machineId),
		queryFn: () => listVolumes(machineId),
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
