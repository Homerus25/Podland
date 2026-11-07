import type {
	ConnectContainerVariables,
	CreateNetworkInput,
} from "@features/networks/services/networksApi";
import {
	bulkRemoveNetworks,
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
import type { ApiError, BulkRemoveInput } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

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

// A bulk remove changes the host's network set, so the list refetches on
// success — including after a partial run, where the successes are gone and
// the refused networks stay. It invalidates both lists for the same reason
// the single-item delete does: a forced removal takes the attached
// containers with it, and the containers DTO carries each container's
// networks. The report is the mutation's own data, so the dialog reads it
// straight off the result.
export function useBulkRemoveNetworks(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		NETWORK_MUTATION_INVALIDATIONS,
		(input: BulkRemoveInput) => bulkRemoveNetworks(hostId, input),
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

// The sequential multi-connect runner behind the containers list's bulk
// connect (spec #486): one request at a time in the caller's order, each
// item's outcome reported as it settles, both lists invalidated once at the
// end. The parallel allSettled mutation above stays the two single-fixed-side
// dialogs' transport.
//
// It is a plain hook over the service function, not a react-query mutation:
// the bulk flow narrates per item as it goes (a mutation reports one
// aggregate result at the end), and the caller owns the report it builds from
// `onItem`. A failed pair never aborts the batch — the remaining pairs still
// fire — and the invalidation runs once after the last pair settles, success
// or failure alike, so succeeded items leave the eligible options and failed
// ones remain for retry.
//
// The optional signal is the batch dialog's abort seam (spec 486 review):
// an aborted run stops before the next pair fires — the pairs that already
// settled are real, so the invalidation still runs, but the caller learns
// nothing further through `onItem` it does not already hold. The dialog
// closes on abort and reports nothing back through its own seam, the same
// abandoned-run rule the batch transfer dialog pins.
export function useSequentialConnect(hostId: string): {
	run: (
		pairs: ConnectContainerVariables[],
		onItem: (containerId: string, failure: ApiError | undefined) => void,
		signal?: AbortSignal,
	) => Promise<void>;
} {
	const queryClient = useQueryClient();

	const run = useCallback(
		async (
			pairs: ConnectContainerVariables[],
			onItem: (containerId: string, failure: ApiError | undefined) => void,
			signal?: AbortSignal,
		) => {
			for (const { networkName, containerId } of pairs) {
				if (signal?.aborted) break;
				try {
					await connectContainer(hostId, networkName, containerId, signal);
					onItem(containerId, undefined);
				} catch (error) {
					onItem(containerId, getApiError(error));
				}
			}
			invalidateHostResources(
				queryClient,
				hostId,
				NETWORK_MUTATION_INVALIDATIONS,
			);
		},
		[hostId, queryClient],
	);

	return { run };
}
