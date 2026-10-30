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

export function useSecrets(machineId: string) {
	return useAlwaysRefetchListQuery(secretsQueryKey(machineId), () =>
		listSecrets(machineId),
	);
}

// A create extends the machine's secret list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateSecret(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"secrets",
		(input: CreateSecretInput) => createSecret(machineId, input),
	);
}

export function useSecret(machineId: string, secretId: string) {
	return useOneShotQuery(secretQueryKey(machineId, secretId), () =>
		getSecret(machineId, secretId),
	);
}

export function useSecretInspect(
	machineId: string,
	secretId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		secretInspectQueryKey(machineId, secretId),
		() => inspectSecret(machineId, secretId),
		options,
	);
}

// The revealed value travels through a mutation so React Query never caches
// it; it lives only in the component that owns the mutation and is dropped
// when the detail view closes.
export function useRevealSecret(machineId: string) {
	return useMutation({
		mutationFn: (secretId: string) => getSecret(machineId, secretId, true),
	});
}

// A delete removes the secret from the machine, so the list refetches on
// success; no optimistic updates. Errors (409 SECRET_IN_USE) surface in the
// confirmation modal that owns the mutation.
export function useDeleteSecret(machineId: string) {
	return useListInvalidatingMutation(machineId, "secrets", (secretId: string) =>
		deleteSecret(machineId, secretId),
	);
}

// The transfer dialog's fact payload, keyed by the chosen target so a
// target change refetches. The ritual — the null-target guard, the
// empty-string placeholder key, the enabled gate — is the shared
// transfer-hooks module's (issue 273); this adapter supplies the key
// constructor and fetcher.
export function useSecretMovePreview(
	machineId: string,
	secretId: string,
	targetMachineId: string | null,
) {
	return useMovePreview(
		(target) => secretMovePreviewQueryKey(machineId, secretId, target),
		(target) => getSecretMovePreview(machineId, secretId, target),
		targetMachineId,
	);
}

// A secret transfer touches two machines: the target gains the secret and
// (in move mode) the source loses it. The plain mutation behind the
// transfer dialog answers one JSON envelope (no stream), and onSuccess
// invalidates both machines' secret lists from what the server answered
// rather than from closure assumptions — the both-machines rule the shared
// transfer-hooks module owns (issue 273), called from this onSuccess and
// from the stream factory's complete handler alike. A "partial" result is
// still a success-shaped answer — the value crossed — so both lists refresh
// there too.
export function useMoveSecret(machineId: string) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: ({
			secretId,
			...input
		}: { secretId: string } & SecretMoveInput) =>
			moveSecret(machineId, secretId, input),
		onSuccess: (result) => {
			invalidateMoveResult(queryClient, result, "secrets");
		},
	});
}
