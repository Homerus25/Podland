import type {
	CreateQuadletInput,
	DeleteQuadletOptions,
	QuadletLifecycleOptions,
	UpdateQuadletInput,
} from "@features/quadlets/services/quadletsApi";
import {
	createQuadlet,
	deleteQuadlet,
	getQuadlet,
	listQuadlets,
	restartQuadlet,
	startQuadlet,
	stopQuadlet,
	updateQuadlet,
} from "@features/quadlets/services/quadletsApi";
import { quadletQueryKey, quadletsQueryKey } from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function useQuadlets(machineId: string) {
	return useQuery({
		queryKey: quadletsQueryKey(machineId),
		queryFn: () => listQuadlets(machineId),
	});
}

export function useCreateQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		(input: CreateQuadletInput) => createQuadlet(machineId, input),
	);
}

export function useQuadlet(machineId: string, quadletId: string) {
	return useQuery({
		queryKey: quadletQueryKey(machineId, quadletId),
		queryFn: () => getQuadlet(machineId, quadletId),
		// The editor must open on the quadlet's current content, so the detail
		// cache is dropped as soon as the modal unmounts: reopening (e.g. after
		// a save) refetches instead of reseeding the form with the pre-edit file.
		gcTime: 0,
	});
}

// A content update can change the quadlet's systemd state, so the list
// refetches on success.
export function useUpdateQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		({ quadletId, content }: { quadletId: string } & UpdateQuadletInput) =>
			updateQuadlet(machineId, quadletId, { content }),
	);
}

// A delete removes the unit from the machine, so the list refetches on
// success and the quadlet disappears.
export function useDeleteQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		({ quadletId, ...options }: { quadletId: string } & DeleteQuadletOptions) =>
			deleteQuadlet(machineId, quadletId, options),
	);
}

// Lifecycle actions change the unit's systemd state, so the list refetches on
// success and the state badge reflects what the machine now reports.
export function useStartQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		(quadletId: string) => startQuadlet(machineId, quadletId),
	);
}

export function useStopQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		({
			quadletId,
			...options
		}: { quadletId: string } & QuadletLifecycleOptions) =>
			stopQuadlet(machineId, quadletId, options),
	);
}

export function useRestartQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		({
			quadletId,
			...options
		}: { quadletId: string } & QuadletLifecycleOptions) =>
			restartQuadlet(machineId, quadletId, options),
	);
}
