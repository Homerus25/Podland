import type { CreateContainerInput } from "@features/containers/services/containersApi";
import {
	createContainer,
	deleteContainer,
	listContainers,
	startContainer,
	stopContainer,
} from "@features/containers/services/containersApi";
import { containersQueryKey } from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useContainers(machineId: string) {
	return useQuery({
		queryKey: containersQueryKey(machineId),
		queryFn: () => listContainers(machineId),
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
