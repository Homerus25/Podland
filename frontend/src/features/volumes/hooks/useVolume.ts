import { inspectVolume } from "@features/volumes/services/volumesApi";
import { volumeQueryKey } from "@shared/queries/queryKeys";
import { useOneShotQuery } from "@shared/queries/useFreshQuery";

// The volume's inspect response is also its detail data, so the detail
// page's "Show inspect JSON" reveal rides this hook with enabled: revealed.
export function useVolume(
	hostId: string,
	volumeName: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		volumeQueryKey(hostId, volumeName),
		() => inspectVolume(hostId, volumeName),
		options,
	);
}
