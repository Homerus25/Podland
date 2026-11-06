import type {
	CreateContainerInput,
	UpdateContainerInput,
} from "@features/containers/services/containersApi";
import {
	bulkRemoveContainers,
	createContainer,
	deleteContainer,
	getContainer,
	getContainerStats,
	inspectContainer,
	listContainerStats,
	listContainers,
	pruneContainers,
	renameContainer,
	restartContainer,
	startContainer,
	stopContainer,
	updateContainer,
} from "@features/containers/services/containersApi";
import {
	containerInspectQueryKey,
	containerQueryKey,
	containerStatsQueryKey,
	containersQueryKey,
	containersStatsQueryKey,
} from "@shared/queries/queryKeys";
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import {
	invalidateHostResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import type { BulkRemoveInput } from "@shared/types/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export function useContainers(hostId: string) {
	return useAlwaysRefetchListQuery(containersQueryKey(hostId), () =>
		listContainers(hostId),
	);
}

export function useContainer(hostId: string, containerId: string) {
	return useOneShotQuery(containerQueryKey(hostId, containerId), () =>
		getContainer(hostId, containerId),
	);
}

export function useContainerInspect(
	hostId: string,
	containerId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containerInspectQueryKey(hostId, containerId),
		() => inspectContainer(hostId, containerId),
		options,
	);
}

export function useContainerStatsList(
	hostId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containersStatsQueryKey(hostId),
		() => listContainerStats(hostId),
		options,
	);
}

export function useContainerStats(
	hostId: string,
	containerId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containerStatsQueryKey(hostId, containerId),
		() => getContainerStats(hostId, containerId),
		options,
	);
}

export function useStartContainer(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(containerId: string) => startContainer(hostId, containerId),
	);
}

export function useStopContainer(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(containerId: string) => stopContainer(hostId, containerId),
	);
}

export function useRestartContainer(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(containerId: string) => restartContainer(hostId, containerId),
	);
}

// A rename changes the container's name, which surfaces in three places:
// the containers list, the container's own detail query (the detail key
// extends the containers key, so the invalidation's prefix match covers
// it), and the pods list (pod data carries its containers' names).
export function useRenameContainer(hostId: string, containerId: string) {
	return useListInvalidatingMutation(
		hostId,
		["containers", "pods"],
		(name: string) => renameContainer(hostId, containerId, name),
	);
}

// A configuration update changes the container's own detail data (limits
// and healthcheck) and nothing else: the containers list and the detail and
// inspect queries refetch (the detail and inspect keys extend the
// containers key, so the invalidation's prefix match covers them).
export function useUpdateContainer(hostId: string, containerId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(input: UpdateContainerInput) =>
			updateContainer(hostId, containerId, input),
	);
}

// A delete removes the container from the host, so the list refetches on
// success. The variables carry the force flag: the first confirm sends the
// plain request, and only a 409 CONTAINER_RUNNING refusal earns the forced
// retry (the force-refusal preset keys the flag off the current error).
export function useDeleteContainer(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		({ containerId, force }: { containerId: string; force?: boolean }) =>
			deleteContainer(hostId, containerId, force ?? false),
	);
}

// A bulk remove takes several containers off the host in one request, so
// the list refetches on success. The per-entry report rides the mutation's
// data, not the cache.
export function useBulkRemoveContainers(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(input: BulkRemoveInput) => bulkRemoveContainers(hostId, input),
	);
}

// A prune changes the host's container set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneContainers(hostId: string) {
	return useListInvalidatingMutation(hostId, "containers", () =>
		pruneContainers(hostId),
	);
}

export function useCreateContainer(hostId: string) {
	const queryClient = useQueryClient();
	// Hand-rolled instead of useListInvalidatingMutation (spec 356): the
	// create additionally invalidates the pods list when the container
	// joins one — a create that joins a pod changes the pods list's
	// container counts (spec 352), so only then does the pods list refetch.
	return useMutation({
		mutationFn: (input: CreateContainerInput) => createContainer(hostId, input),
		onSuccess: (_created, input) => {
			invalidateHostResources(queryClient, hostId, "containers");
			if (input.pod) {
				invalidateHostResources(queryClient, hostId, "pods");
			}
		},
	});
}
