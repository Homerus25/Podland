import { listVolumes } from "@features/volumes/services/volumesApi";
import { useQuery } from "@tanstack/react-query";

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: ["machines", machineId, "volumes"],
		queryFn: () => listVolumes(machineId),
	});
}
