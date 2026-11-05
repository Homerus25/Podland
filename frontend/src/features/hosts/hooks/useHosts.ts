import {
	type CreateHostInput,
	createHost,
	deleteHost,
	getHost,
	getHostStatus,
	getHostVersion,
	grantHostAccess,
	listHostAccess,
	listHosts,
	revokeHostAccess,
	type UpdateHostInput,
	updateHost,
} from "@features/hosts/services/hostsApi";
import {
	hostAccessQueryKey,
	hostDetailKey,
	hostStatusQueryKey,
	hostsQueryKey,
	hostVersionQueryKey,
} from "@shared/queries/queryKeys";
import { useAlwaysRefetchListQuery } from "@shared/queries/useFreshQuery";
import type { HostPermission } from "@shared/types/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export function useHosts() {
	return useQuery({
		queryKey: hostsQueryKey,
		queryFn: listHosts,
	});
}

// Per-host live probe. staleTime 0 and no refetch interval: status is
// fresh on every mount of the overview and never polled (no-polling
// precedent). Query key includes the host id so each card probes
// independently and the hosts list is never blocked.
export function useHostStatus(hostId: string) {
	return useQuery({
		queryKey: hostStatusQueryKey(hostId),
		queryFn: () => getHostStatus(hostId),
		staleTime: 0,
	});
}

// Remote Podman's version report for the detail page header. Mirrors the
// status probe's staleness pattern exactly: staleTime 0 and no refetch
// interval — the version is fresh on every mount of the page and never
// polled.
export function useHostVersion(hostId: string) {
	return useQuery({
		queryKey: hostVersionQueryKey(hostId),
		queryFn: () => getHostVersion(hostId),
		staleTime: 0,
	});
}

export function useHost(id: string) {
	return useQuery({
		queryKey: hostDetailKey(id),
		queryFn: () => getHost(id),
	});
}

// A host change touches every query under the hosts prefix, so these
// mutations invalidate the whole family deliberately (unlike resource-list
// mutations, which target a single resource list).
export function useUpdateHost(id: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: UpdateHostInput) => updateHost(id, input),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: hostsQueryKey });
		},
	});
}

export function useDeleteHost() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (id: string) => deleteHost(id),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: hostsQueryKey });
		},
	});
}

export function useHostAccess(hostId: string) {
	return useAlwaysRefetchListQuery(hostAccessQueryKey(hostId), () =>
		listHostAccess(hostId),
	);
}

// Sharing changes the host's access list, so the access query refetches
// on success.
function useInvalidateSharing(hostId: string) {
	const queryClient = useQueryClient();
	return () => {
		queryClient.invalidateQueries({
			queryKey: hostAccessQueryKey(hostId),
		});
	};
}

export function useGrantHostAccess(hostId: string) {
	const invalidate = useInvalidateSharing(hostId);
	return useMutation({
		mutationFn: ({
			userId,
			permission,
		}: {
			userId: string;
			permission: HostPermission;
		}) => grantHostAccess(hostId, userId, permission),
		onSuccess: invalidate,
	});
}

export function useRevokeHostAccess(hostId: string) {
	const invalidate = useInvalidateSharing(hostId);
	return useMutation({
		mutationFn: (userId: string) => revokeHostAccess(hostId, userId),
		onSuccess: invalidate,
	});
}

export function useCreateHost() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: CreateHostInput) => createHost(input),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: hostsQueryKey });
		},
	});
}
