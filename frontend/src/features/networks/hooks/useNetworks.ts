import { listNetworks } from "@features/networks/services/networksApi";
import { networksQueryKey } from "@shared/queries/queryKeys";
import { useQuery } from "@tanstack/react-query";

export function useNetworks(machineId: string) {
	return useQuery({
		queryKey: networksQueryKey(machineId),
		queryFn: () => listNetworks(machineId),
	});
}
