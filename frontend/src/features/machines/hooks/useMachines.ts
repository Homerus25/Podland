import {
	type CreateMachineInput,
	createMachine,
	deleteMachine,
	getMachine,
	getMachineStatus,
	getMachineVersion,
	grantMachineAccess,
	listMachineAccess,
	listMachines,
	revokeMachineAccess,
	type UpdateMachineInput,
	updateMachine,
} from "@features/machines/services/machinesApi";
import {
	machineAccessQueryKey,
	machineDetailKey,
	machineStatusQueryKey,
	machinesQueryKey,
	machineVersionQueryKey,
} from "@shared/queries/queryKeys";
import { useAlwaysRefetchListQuery } from "@shared/queries/useFreshQuery";
import type { MachinePermission } from "@shared/types/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export function useMachines() {
	return useQuery({
		queryKey: machinesQueryKey,
		queryFn: listMachines,
	});
}

// Per-machine live probe. staleTime 0 and no refetch interval: status is
// fresh on every mount of the overview and never polled (no-polling
// precedent). Query key includes the machine id so each card probes
// independently and the machines list is never blocked.
export function useMachineStatus(machineId: string) {
	return useQuery({
		queryKey: machineStatusQueryKey(machineId),
		queryFn: () => getMachineStatus(machineId),
		staleTime: 0,
	});
}

// Remote Podman's version report for the detail page header. Mirrors the
// status probe's staleness pattern exactly: staleTime 0 and no refetch
// interval — the version is fresh on every mount of the page and never
// polled.
export function useMachineVersion(machineId: string) {
	return useQuery({
		queryKey: machineVersionQueryKey(machineId),
		queryFn: () => getMachineVersion(machineId),
		staleTime: 0,
	});
}

export function useMachine(id: string) {
	return useQuery({
		queryKey: machineDetailKey(id),
		queryFn: () => getMachine(id),
	});
}

// A machine change touches every query under the machines prefix, so these
// mutations invalidate the whole family deliberately (unlike resource-list
// mutations, which target a single resource list).
export function useUpdateMachine(id: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: UpdateMachineInput) => updateMachine(id, input),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: machinesQueryKey });
		},
	});
}

export function useDeleteMachine() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (id: string) => deleteMachine(id),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: machinesQueryKey });
		},
	});
}

export function useMachineAccess(machineId: string) {
	return useAlwaysRefetchListQuery(machineAccessQueryKey(machineId), () =>
		listMachineAccess(machineId),
	);
}

// Sharing changes the machine's access list, so the access query refetches
// on success.
function useInvalidateSharing(machineId: string) {
	const queryClient = useQueryClient();
	return () => {
		queryClient.invalidateQueries({
			queryKey: machineAccessQueryKey(machineId),
		});
	};
}

export function useGrantMachineAccess(machineId: string) {
	const invalidate = useInvalidateSharing(machineId);
	return useMutation({
		mutationFn: ({
			userId,
			permission,
		}: {
			userId: string;
			permission: MachinePermission;
		}) => grantMachineAccess(machineId, userId, permission),
		onSuccess: invalidate,
	});
}

export function useRevokeMachineAccess(machineId: string) {
	const invalidate = useInvalidateSharing(machineId);
	return useMutation({
		mutationFn: (userId: string) => revokeMachineAccess(machineId, userId),
		onSuccess: invalidate,
	});
}

export function useCreateMachine() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: (input: CreateMachineInput) => createMachine(input),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: machinesQueryKey });
		},
	});
}
