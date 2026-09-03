import { listVolumes } from "@features/volumes/services/volumesApi";
import { useQuery } from "@tanstack/react-query";

export function volumesQueryKey(machineId: string) {
	return ["machines", machineId, "volumes"] as const;
}

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: volumesQueryKey(machineId),
		queryFn: () => listVolumes(machineId),
	});
}
