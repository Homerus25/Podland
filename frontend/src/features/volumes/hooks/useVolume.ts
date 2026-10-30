import { inspectVolume } from "@features/volumes/services/volumesApi";
import { volumeQueryKey } from "@shared/queries/queryKeys";
import { useOneShotQuery } from "@shared/queries/useFreshQuery";

// The volume's inspect response is also its detail data, so the detail
// page's "Show inspect JSON" reveal rides this hook with enabled: revealed.
export function useVolume(
	machineId: string,
	volumeName: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		volumeQueryKey(machineId, volumeName),
		() => inspectVolume(machineId, volumeName),
		options,
	);
}
