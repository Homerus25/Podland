import type { CreateContainerInput } from "@features/containers/services/containersApi";
import {
	createContainer,
	deleteContainer,
	getContainer,
	inspectContainer,
	listContainers,
	startContainer,
	stopContainer,
} from "@features/containers/services/containersApi";
import {
	containerInspectQueryKey,
	containerQueryKey,
	containersQueryKey,
} from "@shared/queries/queryKeys";
import {
	invalidateMachineResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export function useContainers(machineId: string) {
	return useQuery({
		queryKey: containersQueryKey(machineId),
		queryFn: () => listContainers(machineId),
		// Lazy tabs remount the panel on every activation; refetching on
		// mount makes each activation issue a fresh request, even inside the
		// app-wide five-minute stale window.
		refetchOnMount: "always",
	});
}

// The container detail view behind the detail page (spec 301): a singular
// sub-resource keyed by the container, gcTime: 0 so every page visit
// refetches the container's current state.
export function useContainer(machineId: string, containerId: string) {
	return useQuery({
		queryKey: containerQueryKey(machineId, containerId),
		queryFn: () => getContainer(machineId, containerId),
		gcTime: 0,
	});
}

// The raw inspect report behind the detail page's "Show inspect JSON"
// reveal. gcTime: 0 drops the cache entry the moment the page unmounts, so
// every reveal refetches the current report. The detail page passes
// enabled: revealed so the report fetches lazily on the first "Show
// inspect JSON" click, not on page mount.
export function useContainerInspect(
	machineId: string,
	containerId: string,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey: containerInspectQueryKey(machineId, containerId),
		queryFn: () => inspectContainer(machineId, containerId),
		enabled: options?.enabled ?? true,
		gcTime: 0,
	});
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

export function useDeleteContainer(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(containerId: string) => deleteContainer(machineId, containerId),
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
