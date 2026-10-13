import type {
	CreateQuadletInput,
	DeleteQuadletOptions,
	QuadletLifecycleOptions,
	QuadletMoveInput,
	UpdateQuadletInput,
} from "@features/quadlets/services/quadletsApi";
import {
	createQuadlet,
	deleteQuadlet,
	getQuadlet,
	listQuadlets,
	moveQuadlet,
	quadletMovePreview,
	restartQuadlet,
	startQuadlet,
	stopQuadlet,
	updateQuadlet,
} from "@features/quadlets/services/quadletsApi";
import {
	quadletMovePreviewQueryKey,
	quadletQueryKey,
	quadletsQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import {
	invalidateMoveResult,
	useMovePreview,
} from "@shared/transfers/transferHooks";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

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

// The transfer dialog's warning payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useQuadletMovePreview(
	machineId: string,
	quadletId: string,
	targetMachineId: string | null,
) {
	return useMovePreview(
		(target) => quadletMovePreviewQueryKey(machineId, quadletId, target),
		(target) => quadletMovePreview(machineId, quadletId, target),
		targetMachineId,
	);
}

// A move/copy touches two machines: the quadlet leaves the source and (in
// move mode) is gone from it, while the target gains it. The response carries
// both machine ids, so both lists invalidate from what the server answered
// rather than from closure assumptions — the both-machines rule the shared
// transfer-hooks module owns (issue 273), called from this onSuccess and from
// the stream factory's complete handler alike. A "partial" result is still a
// success-shaped answer — the target install stands — so both lists refresh
// there too.
export function useMoveQuadlet(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			quadletId,
			...input
		}: { quadletId: string } & QuadletMoveInput) =>
			moveQuadlet(machineId, quadletId, input),
		onSuccess: (result) => {
			invalidateMoveResult(queryClient, result, "quadlets");
		},
	});
}
