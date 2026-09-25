import {
	type CreateMachineInput,
	createMachine,
	deleteMachine,
	getMachine,
	getMachineStatus,
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
} from "@shared/queries/queryKeys";
import type { MachinePermission } from "@shared/types/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

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

// The backend populates the caller's per-caller facts — `permission` and
// `isOwner` — only on the list endpoint (backend/internal/domain/machines
// /service.go MachineListResponse); the detail response omits both. So the
// list query is consulted to fill in the facts the detail page's gating
// reads (badges, resource tabs, settings).
export function useMachine(id: string) {
	const detail = useQuery({
		queryKey: machineDetailKey(id),
		queryFn: () => getMachine(id),
	});
	const { data: machines } = useMachines();

	return useMemo(() => {
		const listed = machines?.find((machine) => machine.id === id);
		if (!detail.data || !listed?.permission) return detail;
		return {
			...detail,
			data: {
				...detail.data,
				permission: listed.permission,
				isOwner: listed.isOwner,
			},
		};
	}, [detail, machines, id]);
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
	return useQuery({
		queryKey: machineAccessQueryKey(machineId),
		queryFn: () => listMachineAccess(machineId),
	});
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
