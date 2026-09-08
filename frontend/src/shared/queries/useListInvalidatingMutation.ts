import type { MachineResource } from "@shared/queries/queryKeys";
import {
	containersQueryKey,
	imagesQueryKey,
	quadletsQueryKey,
	secretsQueryKey,
	volumesQueryKey,
} from "@shared/queries/queryKeys";
import type { QueryClient } from "@tanstack/react-query";
import { useMutation, useQueryClient } from "@tanstack/react-query";

const resourceKeys: Record<
	MachineResource,
	(machineId: string) => readonly unknown[]
> = {
	containers: containersQueryKey,
	volumes: volumesQueryKey,
	images: imagesQueryKey,
	quadlets: quadletsQueryKey,
	secrets: secretsQueryKey,
};

// Fire-and-forget invalidation for flows outside the mutation factory:
// streaming completion handlers and error-driven refreshes.
export function invalidateMachineResources(
	queryClient: QueryClient,
	machineId: string,
	resources: MachineResource | MachineResource[],
): void {
	const targets = Array.isArray(resources) ? resources : [resources];
	for (const resource of targets) {
		queryClient.invalidateQueries({
			queryKey: resourceKeys[resource](machineId),
		});
	}
}

// Mutations that change a machine's resource list invalidate that list on
// success so it refetches the server's post-action state; no optimistic
// updates. The useMutation result passes through untouched, so isPending,
// error, mutate, and mutationToPromise all keep working.
export function useListInvalidatingMutation<TInput = void, TData = unknown>(
	machineId: string,
	resources: MachineResource | MachineResource[],
	run: (input: TInput) => Promise<TData>,
) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: run,
		onSuccess: () =>
			invalidateMachineResources(queryClient, machineId, resources),
	});
}
