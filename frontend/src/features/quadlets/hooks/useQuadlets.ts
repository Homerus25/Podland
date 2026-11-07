import type {
	CreateQuadletInput,
	GenerateQuadletInput,
	QuadletImportRequest,
	QuadletLifecycleOptions,
	QuadletMoveInput,
	UpdateQuadletInput,
} from "@features/quadlets/services/quadletsApi";
import {
	bulkRemoveQuadlets,
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
	useMovePreviews,
} from "@shared/transfers/transferHooks";
import type { BulkRemoveInput } from "@shared/types/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export function useQuadlets(hostId: string) {
	return useAlwaysRefetchListQuery(quadletsQueryKey(hostId), () =>
		listQuadlets(hostId),
	);
}

export function useCreateQuadlet(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		(input: CreateQuadletInput) => createQuadlet(hostId, input),
	);
}

// Generating a quadlet from an existing resource installs nothing on the
// host, so it rides a plain mutation (spec #402 ticket #406); the save
// that follows is a regular create through useCreateQuadlet.
export function useGenerateQuadlet(hostId: string) {
	return useMutation({
		mutationFn: (input: GenerateQuadletInput) => generateQuadlet(hostId, input),
	});
}

// One-shot: the editor must reopen on the file's current content, so a
// reopen after a save refetches instead of reseeding the form with the
// pre-edit file.
export function useQuadlet(hostId: string, quadletId: string) {
	return useOneShotQuery(quadletQueryKey(hostId, quadletId), () =>
		getQuadlet(hostId, quadletId),
	);
}

// A content update can change the quadlet's systemd state, so the list
// refetches on success.
export function useUpdateQuadlet(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		({ quadletId, content }: { quadletId: string } & UpdateQuadletInput) =>
			updateQuadlet(hostId, quadletId, { content }),
	);
}

// A bulk remove takes several units off the host in one request, so the
// list refetches on success and the removed quadlets disappear. The
// per-entry report rides the mutation's data, not the cache.
export function useBulkRemoveQuadlets(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		(input: BulkRemoveInput) => bulkRemoveQuadlets(hostId, input),
	);
}

// A single-quadlet remove takes the unit off the host, so the list
// refetches on success (issue #472). The detail page rides the endpoint's
// defaults — stop=true, force=false — so the mutation's variable is the
// bare quadlet id.
export function useDeleteQuadlet(hostId: string) {
	return useListInvalidatingMutation(hostId, "quadlets", (quadletId: string) =>
		deleteQuadlet(hostId, quadletId),
	);
}

// Lifecycle actions change the unit's systemd state, so the list refetches on
// success and the state badge reflects what the host now reports.
export function useStartQuadlet(hostId: string) {
	return useListInvalidatingMutation(hostId, "quadlets", (quadletId: string) =>
		startQuadlet(hostId, quadletId),
	);
}

export function useStopQuadlet(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		({
			quadletId,
			...options
		}: { quadletId: string } & QuadletLifecycleOptions) =>
			stopQuadlet(hostId, quadletId, options),
	);
}

export function useRestartQuadlet(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		({
			quadletId,
			...options
		}: { quadletId: string } & QuadletLifecycleOptions) =>
			restartQuadlet(hostId, quadletId, options),
	);
}

// The transfer dialog's warning payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useQuadletMovePreview(
	hostId: string,
	quadletId: string,
	targetHostId: string | null,
) {
	return useMovePreview(
		(target) => quadletMovePreviewQueryKey(hostId, quadletId, target),
		(target) => quadletMovePreview(hostId, quadletId, target),
		targetHostId,
	);
}

// The batch transfer's previews (spec 486, ticket #489): one query per
// selected quadlet, over the SAME key constructor and fetcher the single-item
// hook uses, so a batch and a single transfer share cache entries. The
// ritual is the shared transfer-hooks module's (useMovePreviews); this
// adapter supplies the key constructor and fetcher.
export function useQuadletMovePreviews(
	hostId: string,
	quadletIds: string[],
	targetHostId: string | null,
) {
	return useMovePreviews(
		(quadletId, target) =>
			quadletMovePreviewQueryKey(hostId, quadletId, target),
		(quadletId, target) => quadletMovePreview(hostId, quadletId, target),
		quadletIds,
		targetHostId,
	);
}

// Compose-to-quadlet conversion touches no host data, so it rides a
// plain mutation — never a cached query (spec #336).
export function useConvertCompose() {
	return useMutation({
		mutationFn: (content: string) => convertCompose(content),
	});
}

// Installing the converted quadlets changes the host's quadlet list, so
// the list refetches on success (spec #336).
export function useImportQuadlets(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"quadlets",
		(request: QuadletImportRequest) => importQuadlets(hostId, request),
	);
}

// A move/copy touches two hosts: the quadlet leaves the source and (in
// move mode) is gone from it, while the target gains it. The response carries
// both host ids, so both lists invalidate from what the server answered
// rather than from closure assumptions — the both-hosts rule the shared
// transfer-hooks module owns (issue 273), called from this onSuccess and from
// the stream factory's complete handler alike. A "partial" result is still a
// success-shaped answer — the target install stands — so both lists refresh
// there too.
export function useMoveQuadlet(hostId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			quadletId,
			...input
		}: { quadletId: string } & QuadletMoveInput) =>
			moveQuadlet(hostId, quadletId, input),
		onSuccess: (result) => {
			invalidateMoveResult(queryClient, result, "quadlets");
		},
	});
}
