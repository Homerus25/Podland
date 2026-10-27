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
	pruneNetworks,
} from "@features/networks/services/networksApi";
import type { MachineResource } from "@shared/queries/queryKeys";
import {
	networkInspectQueryKey,
	networkQueryKey,
	networksQueryKey,
} from "@shared/queries/queryKeys";
import {
	invalidateMachineResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import type { ApiError } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

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

// A prune removes only unused networks — no container is attached to a
// removed network, so the containers list is untouched and only the
// networks list refetches on success.
export function usePruneNetworks(machineId: string) {
	return useListInvalidatingMutation(machineId, "networks", () =>
		pruneNetworks(machineId),
	);
}

// One pair that failed among a multi-connect batch (issue 320): the pair
// plus the display-ready error the backend answered with.
export interface ConnectFailure {
	networkName: string;
	containerId: string;
	error: ApiError;
}

// The aggregated error of a multi-connect batch: thrown when any request
// fails, carrying every failed pair so the dialog can name them. All
// requests fire regardless of individual failures; succeeded connections
// stay connected (no rollback).
export class ConnectError extends Error {
	readonly failures: ConnectFailure[];
	constructor(failures: ConnectFailure[]) {
		super(`${failures.length} connection(s) failed`);
		this.name = "ConnectError";
		this.failures = failures;
	}
}

// The multi-connect mutation (issue 320): one connectContainer call per
// pair via Promise.allSettled, so every request fires regardless of
// individual failures. Both lists invalidate once, after all requests
// settle — success or partial failure alike — so succeeded items leave
// the eligible options and failed ones remain for retry. Any failure
// rejects the mutation with the aggregated ConnectError, which keeps the
// dialog open; only an all-success batch resolves and closes it.
export function useConnectContainers(machineId: string) {
	const queryClient = useQueryClient();
	// TError is ConnectError exactly: the mutationFn's only throw is the
	// aggregated batch error — individual request failures are captured by
	// allSettled and folded into it, never rethrown raw.
	return useMutation<void, ConnectError, ConnectContainerVariables[]>({
		mutationFn: async (pairs: ConnectContainerVariables[]) => {
			const results = await Promise.allSettled(
				pairs.map(({ networkName, containerId }) =>
					connectContainer(machineId, networkName, containerId),
				),
			);
			const failures: ConnectFailure[] = [];
			for (const [index, result] of results.entries()) {
				if (result.status === "rejected") {
					failures.push({
						...pairs[index],
						error: getApiError(result.reason),
					});
				}
			}
			if (failures.length > 0) {
				throw new ConnectError(failures);
			}
		},
		onSettled: () =>
			invalidateMachineResources(
				queryClient,
				machineId,
				NETWORK_MUTATION_INVALIDATIONS,
			),
	});
}
