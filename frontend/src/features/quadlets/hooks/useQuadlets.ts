import type {
	CreateQuadletInput,
	DeleteQuadletOptions,
	GenerateQuadletInput,
	QuadletImportRequest,
	QuadletLifecycleOptions,
	QuadletMoveInput,
	UpdateQuadletInput,
} from "@features/quadlets/services/quadletsApi";
import {
	convertCompose,
	createQuadlet,
	deleteQuadlet,
	generateQuadlet,
	getQuadlet,
	importQuadlets,
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
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import {
	invalidateMoveResult,
	useMovePreview,
} from "@shared/transfers/transferHooks";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export function useQuadlets(machineId: string) {
	return useAlwaysRefetchListQuery(quadletsQueryKey(machineId), () =>
		listQuadlets(machineId),
	);
}

export function useCreateQuadlet(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		(input: CreateQuadletInput) => createQuadlet(machineId, input),
	);
}

// Generating a quadlet from an existing resource installs nothing on the
// machine, so it rides a plain mutation (spec #402 ticket #406); the save
// that follows is a regular create through useCreateQuadlet.
export function useGenerateQuadlet(machineId: string) {
	return useMutation({
		mutationFn: (input: GenerateQuadletInput) =>
			generateQuadlet(machineId, input),
	});
}

// One-shot: the editor must reopen on the file's current content, so a
// reopen after a save refetches instead of reseeding the form with the
// pre-edit file.
export function useQuadlet(machineId: string, quadletId: string) {
	return useOneShotQuery(quadletQueryKey(machineId, quadletId), () =>
		getQuadlet(machineId, quadletId),
	);
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

// Compose-to-quadlet conversion touches no machine data, so it rides a
// plain mutation — never a cached query (spec #336).
export function useConvertCompose() {
	return useMutation({
		mutationFn: (content: string) => convertCompose(content),
	});
}

// Installing the converted quadlets changes the machine's quadlet list, so
// the list refetches on success (spec #336).
export function useImportQuadlets(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"quadlets",
		(request: QuadletImportRequest) => importQuadlets(machineId, request),
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
