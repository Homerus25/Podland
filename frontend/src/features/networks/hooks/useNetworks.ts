import type {
	ConnectContainerVariables,
	CreateNetworkInput,
} from "@features/networks/services/networksApi";
import {
	connectContainer,
	createNetwork,
	listNetworks,
} from "@features/networks/services/networksApi";
import type { MachineResource } from "@shared/queries/queryKeys";
import { networksQueryKey } from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useNetworks(machineId: string) {
	return useQuery({
		queryKey: networksQueryKey(machineId),
		queryFn: () => listNetworks(machineId),
	});
}

// Every network mutation invalidates both lists: the containers DTO carries
// each container's networks, so an attach is visible from both tabs.
const NETWORK_MUTATION_INVALIDATIONS: MachineResource[] = [
	"networks",
	"containers",
];

export function useCreateNetwork(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		NETWORK_MUTATION_INVALIDATIONS,
		(input: CreateNetworkInput) => createNetwork(machineId, input),
	);
}

export function useConnectContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		NETWORK_MUTATION_INVALIDATIONS,
		({ networkName, containerId }: ConnectContainerVariables) =>
			connectContainer(machineId, networkName, containerId),
	);
}
