import type {
	ConnectContainerVariables,
	CreateNetworkInput,
} from "@features/networks/services/networksApi";
import {
	connectContainer,
	createNetwork,
	deleteNetwork,
	getNetwork,
	inspectNetwork,
	listNetworks,
	pruneNetworks,
} from "@features/networks/services/networksApi";
import type { HostResource } from "@shared/queries/queryKeys";
import {
	networkInspectQueryKey,
	networkQueryKey,
	networksQueryKey,
} from "@shared/queries/queryKeys";
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import {
	invalidateHostResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import type { ApiError } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export function useNetworks(hostId: string) {
	return useAlwaysRefetchListQuery(networksQueryKey(hostId), () =>
		listNetworks(hostId),
	);
}

export function useNetwork(hostId: string, networkName: string) {
	return useOneShotQuery(networkQueryKey(hostId, networkName), () =>
		getNetwork(hostId, networkName),
	);
}

export function useNetworkInspect(
	hostId: string,
	networkName: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		networkInspectQueryKey(hostId, networkName),
		() => inspectNetwork(hostId, networkName),
		options,
	);
}

// Every network mutation invalidates both lists: the containers DTO carries
// each container's networks, so an attach is visible from both tabs.
const NETWORK_MUTATION_INVALIDATIONS: HostResource[] = [
	"networks",
	"containers",
];

export function useCreateNetwork(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		NETWORK_MUTATION_INVALIDATIONS,
		(input: CreateNetworkInput) => createNetwork(hostId, input),
	);
}

// A prune removes only unused networks — no container is attached to a
// removed network, so the containers list is untouched and only the
// networks list refetches on success.
export function usePruneNetworks(hostId: string) {
	return useListInvalidatingMutation(hostId, "networks", () =>
		pruneNetworks(hostId),
	);
}

// A delete removes the network; with force podman also removes the
// containers attached to it (stopping any that are running), so both
// lists refetch — the containers DTO carries each container's networks.
export function useDeleteNetwork(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		NETWORK_MUTATION_INVALIDATIONS,
		({ networkName, force }: { networkName: string; force: boolean }) =>
			deleteNetwork(hostId, networkName, force),
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
export function useConnectContainers(hostId: string) {
	const queryClient = useQueryClient();
	// TError is ConnectError exactly: the mutationFn's only throw is the
	// aggregated batch error — individual request failures are captured by
	// allSettled and folded into it, never rethrown raw.
	return useMutation<void, ConnectError, ConnectContainerVariables[]>({
		mutationFn: async (pairs: ConnectContainerVariables[]) => {
			const results = await Promise.allSettled(
				pairs.map(({ networkName, containerId }) =>
					connectContainer(hostId, networkName, containerId),
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
			invalidateHostResources(
				queryClient,
				hostId,
				NETWORK_MUTATION_INVALIDATIONS,
			),
	});
}
