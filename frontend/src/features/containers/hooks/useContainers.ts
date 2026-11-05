import type {
	CreateContainerInput,
	UpdateContainerInput,
} from "@features/containers/services/containersApi";
import {
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

export function useDeleteContainer(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"containers",
		(containerId: string) => deleteContainer(hostId, containerId),
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
