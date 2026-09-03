import { inspectVolume } from "@features/volumes/services/volumesApi";
import { useQuery } from "@tanstack/react-query";
import { volumesQueryKey } from "./useVolumes";

export function useVolume(machineId: string, volumeName: string | null) {
	return useQuery({
		queryKey: [...volumesQueryKey(machineId), volumeName],
		queryFn: () => inspectVolume(machineId, volumeName as string),
		enabled: volumeName !== null,
		// The drawer must open on the volume's current state; a stale
		// cache would hide deletions that happened since the last open.
		gcTime: 0,
	});
}
