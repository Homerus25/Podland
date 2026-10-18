import { inspectVolume } from "@features/volumes/services/volumesApi";
import { volumeQueryKey } from "@shared/queries/queryKeys";
import { useQuery } from "@tanstack/react-query";

// The volume's inspect response is also its detail data, so the detail
// page's "Show inspect JSON" reveal rides this hook with enabled: revealed
// — the report fetches lazily on the first reveal click, not on page mount.
export function useVolume(
	machineId: string,
	volumeName: string,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey: volumeQueryKey(machineId, volumeName),
		queryFn: () => inspectVolume(machineId, volumeName),
		enabled: options?.enabled ?? true,
		// The drawer must open on the volume's current state; a stale
		// cache would hide deletions that happened since the last open.
		gcTime: 0,
	});
}
