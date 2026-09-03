import {
	createVolume,
	deleteVolume,
	listVolumes,
	populateVolume,
	pruneVolumes,
	type VolumeCreateInput,
} from "@features/volumes/services/volumesApi";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export function volumesQueryKey(machineId: string) {
	return ["machines", machineId, "volumes"] as const;
}

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: volumesQueryKey(machineId),
		queryFn: () => listVolumes(machineId),
	});
}

// A create extends the machine's volume list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateVolume(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: VolumeCreateInput) => createVolume(machineId, input),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: volumesQueryKey(machineId) });
		},
	});
}

// A delete removes one volume from the machine's list, so the list
// refetches on success; no optimistic updates.
export function useDeleteVolume(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			volumeName,
			force,
		}: {
			volumeName: string;
			force: boolean;
		}) => deleteVolume(machineId, volumeName, force),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: volumesQueryKey(machineId) });
		},
	});
}

// A prune changes the machine's volume set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneVolumes(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: () => pruneVolumes(machineId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: volumesQueryKey(machineId) });
		},
	});
}

// Populate changes a volume's contents (and reported size), so the list
// refetches on success; no optimistic updates.
export function usePopulateVolume(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: {
			volumeName: string;
			form: FormData;
			onProgress?: (fraction: number) => void;
		}) =>
			populateVolume(machineId, input.volumeName, input.form, input.onProgress),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: volumesQueryKey(machineId) });
		},
	});
}
