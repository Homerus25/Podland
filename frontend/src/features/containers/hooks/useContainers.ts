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
	invalidateMachineResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export function useContainers(machineId: string) {
	return useAlwaysRefetchListQuery(containersQueryKey(machineId), () =>
		listContainers(machineId),
	);
}

export function useContainer(machineId: string, containerId: string) {
	return useOneShotQuery(containerQueryKey(machineId, containerId), () =>
		getContainer(machineId, containerId),
	);
}

export function useContainerInspect(
	machineId: string,
	containerId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containerInspectQueryKey(machineId, containerId),
		() => inspectContainer(machineId, containerId),
		options,
	);
}

export function useContainerStatsList(
	machineId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containersStatsQueryKey(machineId),
		() => listContainerStats(machineId),
		options,
	);
}

export function useContainerStats(
	machineId: string,
	containerId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		containerStatsQueryKey(machineId, containerId),
		() => getContainerStats(machineId, containerId),
		options,
	);
}

export function useStartContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(containerId: string) => startContainer(machineId, containerId),
	);
}

export function useStopContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(containerId: string) => stopContainer(machineId, containerId),
	);
}

export function useRestartContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(containerId: string) => restartContainer(machineId, containerId),
	);
}

// A rename changes the container's name, which surfaces in three places:
// the containers list, the container's own detail query (the detail key
// extends the containers key, so the invalidation's prefix match covers
// it), and the pods list (pod data carries its containers' names).
export function useRenameContainer(machineId: string, containerId: string) {
	return useListInvalidatingMutation(
		machineId,
		["containers", "pods"],
		(name: string) => renameContainer(machineId, containerId, name),
	);
}

// A configuration update changes the container's own detail data (limits
// and healthcheck) and nothing else: the containers list and the detail and
// inspect queries refetch (the detail and inspect keys extend the
// containers key, so the invalidation's prefix match covers them).
export function useUpdateContainer(machineId: string, containerId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(input: UpdateContainerInput) =>
			updateContainer(machineId, containerId, input),
	);
}

export function useDeleteContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(containerId: string) => deleteContainer(machineId, containerId),
	);
}

// A prune changes the machine's container set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneContainers(machineId: string) {
	return useListInvalidatingMutation(machineId, "containers", () =>
		pruneContainers(machineId),
	);
}

export function useCreateContainer(machineId: string) {
	const queryClient = useQueryClient();
	// Hand-rolled instead of useListInvalidatingMutation (spec 356): the
	// create additionally invalidates the pods list when the container
	// joins one — a create that joins a pod changes the pods list's
	// container counts (spec 352), so only then does the pods list refetch.
	return useMutation({
		mutationFn: (input: CreateContainerInput) =>
			createContainer(machineId, input),
		onSuccess: (_created, input) => {
			invalidateMachineResources(queryClient, machineId, "containers");
			if (input.pod) {
				invalidateMachineResources(queryClient, machineId, "pods");
			}
		},
	});
}
