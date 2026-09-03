import {
	createVolume,
	listVolumes,
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
