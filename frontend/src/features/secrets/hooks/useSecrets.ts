import {
	type CreateSecretInput,
	createSecret,
	deleteSecret,
	getSecret,
	getSecretMovePreview,
	inspectSecret,
	listSecrets,
	moveSecret,
	type SecretMoveInput,
} from "@features/secrets/services/secretsApi";
import {
	secretInspectQueryKey,
	secretMovePreviewQueryKey,
	secretQueryKey,
	secretsQueryKey,
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

export function useSecrets(hostId: string) {
	return useAlwaysRefetchListQuery(secretsQueryKey(hostId), () =>
		listSecrets(hostId),
	);
}

// A create extends the host's secret list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateSecret(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"secrets",
		(input: CreateSecretInput) => createSecret(hostId, input),
	);
}

export function useSecret(hostId: string, secretId: string) {
	return useOneShotQuery(secretQueryKey(hostId, secretId), () =>
		getSecret(hostId, secretId),
	);
}

export function useSecretInspect(
	hostId: string,
	secretId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		secretInspectQueryKey(hostId, secretId),
		() => inspectSecret(hostId, secretId),
		options,
	);
}

// The revealed value travels through a mutation so React Query never caches
// it; it lives only in the component that owns the mutation and is dropped
// when the detail view closes.
export function useRevealSecret(hostId: string) {
	return useMutation({
		mutationFn: (secretId: string) => getSecret(hostId, secretId, true),
	});
}

// A delete removes the secret from the host, so the list refetches on
// success; no optimistic updates. Errors (409 SECRET_IN_USE) surface in the
// confirmation modal that owns the mutation.
export function useDeleteSecret(hostId: string) {
	return useListInvalidatingMutation(hostId, "secrets", (secretId: string) =>
		deleteSecret(hostId, secretId),
	);
}

// The transfer dialog's fact payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useSecretMovePreview(
	hostId: string,
	secretId: string,
	targetHostId: string | null,
) {
	return useMovePreview(
		(target) => secretMovePreviewQueryKey(hostId, secretId, target),
		(target) => getSecretMovePreview(hostId, secretId, target),
		targetHostId,
	);
}

// A secret transfer touches two hosts: the target gains the secret and
// (in move mode) the source loses it. The plain mutation behind the
// transfer dialog answers one JSON envelope (no stream), and onSuccess
// invalidates both hosts' secret lists from what the server answered
// rather than from closure assumptions — the both-hosts rule the shared
// transfer-hooks module owns (issue 273), called from this onSuccess and
// from the stream factory's complete handler alike. A "partial" result is
// still a success-shaped answer — the value crossed — so both lists refresh
// there too.
export function useMoveSecret(hostId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			secretId,
			...input
		}: { secretId: string } & SecretMoveInput) =>
			moveSecret(hostId, secretId, input),
		onSuccess: (result) => {
			invalidateMoveResult(queryClient, result, "secrets");
		},
	});
}
