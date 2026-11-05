import type { HostResource } from "@shared/queries/queryKeys";
import {
	containersQueryKey,
	imagesQueryKey,
	networksQueryKey,
	podsQueryKey,
	quadletsQueryKey,
	secretsQueryKey,
	volumesQueryKey,
} from "@shared/queries/queryKeys";
import type { QueryClient } from "@tanstack/react-query";
import { useMutation, useQueryClient } from "@tanstack/react-query";

const resourceKeys: Record<
	HostResource,
	(hostId: string) => readonly unknown[]
> = {
	containers: containersQueryKey,
	pods: podsQueryKey,
	volumes: volumesQueryKey,
	images: imagesQueryKey,
	networks: networksQueryKey,
	quadlets: quadletsQueryKey,
	secrets: secretsQueryKey,
};

// Fire-and-forget invalidation for flows outside the mutation factory:
// streaming completion handlers and error-driven refreshes.
export function invalidateHostResources(
	queryClient: QueryClient,
	hostId: string,
	resources: HostResource | HostResource[],
): void {
	const targets = Array.isArray(resources) ? resources : [resources];
	for (const resource of targets) {
		queryClient.invalidateQueries({
			queryKey: resourceKeys[resource](hostId),
		});
	}
}

// Mutations that change a host's resource list invalidate that list on
// success so it refetches the server's post-action state; no optimistic
// updates. The useMutation result passes through untouched, so isPending,
// error, mutate, and mutationToPromise all keep working.
export function useListInvalidatingMutation<TInput = void, TData = unknown>(
	hostId: string,
	resources: HostResource | HostResource[],
	run: (input: TInput) => Promise<TData>,
) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: run,
		onSuccess: () => invalidateHostResources(queryClient, hostId, resources),
	});
}
