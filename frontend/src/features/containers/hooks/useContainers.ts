import type { CreateContainerInput } from "@features/containers/services/containersApi";
import {
	createContainer,
	deleteContainer,
	inspectContainer,
	listContainers,
	startContainer,
	stopContainer,
} from "@features/containers/services/containersApi";
import {
	containerInspectQueryKey,
	containersQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

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

// The raw inspect report behind the inspect dialog. gcTime: 0 drops the
// cache entry the moment the dialog unmounts, so every open refetches the
// current report.
export function useContainerInspect(machineId: string, containerId: string) {
	return useQuery({
		queryKey: containerInspectQueryKey(machineId, containerId),
		queryFn: () => inspectContainer(machineId, containerId),
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
	return useListInvalidatingMutation(
		machineId,
		"containers",
		(input: CreateContainerInput) => createContainer(machineId, input),
	);
}
