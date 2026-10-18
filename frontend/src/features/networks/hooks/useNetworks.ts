import type {
	ConnectContainerVariables,
	CreateNetworkInput,
} from "@features/networks/services/networksApi";
import {
	connectContainer,
	createNetwork,
	getNetwork,
	inspectNetwork,
	listNetworks,
} from "@features/networks/services/networksApi";
import type { MachineResource } from "@shared/queries/queryKeys";
import {
	networkInspectQueryKey,
	networkQueryKey,
	networksQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useNetworks(machineId: string) {
	return useQuery({
		queryKey: networksQueryKey(machineId),
		queryFn: () => listNetworks(machineId),
		// Lazy tabs remount the panel on every activation; refetching on
		// mount makes each activation issue a fresh request, even inside the
		// app-wide five-minute stale window.
		refetchOnMount: "always",
	});
}

// The network detail view (spec 301): a network sub-resource keyed by the
// network, gcTime 0 so every page visit refetches and the cache entry dies
// with the page.
export function useNetwork(machineId: string, networkName: string) {
	return useQuery({
		queryKey: networkQueryKey(machineId, networkName),
		queryFn: () => getNetwork(machineId, networkName),
		gcTime: 0,
	});
}

// The detail page passes enabled: revealed, so the report fetches lazily on
// the first "Show inspect JSON" click, never on page mount; gcTime 0 makes
// every reveal refetch.
export function useNetworkInspect(
	machineId: string,
	networkName: string,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey: networkInspectQueryKey(machineId, networkName),
		queryFn: () => inspectNetwork(machineId, networkName),
		enabled: options?.enabled ?? true,
		gcTime: 0,
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
