import {
	useMoveSecret,
	useSecretMovePreviews,
} from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import {
	MultiTransferDialog,
	type MultiTransferItem,
} from "@shared/components/MultiTransferDialog";
import { useTransferState } from "@shared/components/TransferDialog";
import {
	partialOutcome,
	TransferReferencingWarning,
	transferStatusLabel,
} from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";

// The Secrets batch transfer (spec 486, ticket #487): the shared
// multi-item dialog over the existing per-item move endpoint. The adapter
// keeps the secret domain — the manage-secrets capability the target list
// filters on, the per-secret preview queries, the referencing-quadlet
// warning, the plain-mutation transport (the endpoint answers one JSON
// envelope each, so there is no progress stream), and the envelope-to-
// outcome mapping. The both-hosts invalidation stays in the mutation hook,
// where the single-item dialog already leaves it.
//
// The dialog is opened with a snapshot of the selected secrets, so the
// report keeps naming them after the list drops the successes from its
// selection.
export function MoveSecretsDialog({
	host,
	secrets,
	onFinished,
	onClose,
}: {
	host: Host;
	secrets: SecretSummary[];
	onFinished: (succeededKeys: string[]) => void;
	onClose: () => void;
}) {
	const transfer = useTransferState(host, "manage-secrets");
	const previews = useSecretMovePreviews(
		host.id,
		secrets.map((secret) => secret.name),
		transfer.targetId || null,
	);
	const moveSecret = useMoveSecret(host.id);

	const items: MultiTransferItem<SecretSummary>[] = secrets.map(
		(secret, index) => {
			const preview = previews[index];
			return {
				item: secret,
				key: secret.name,
				label: secret.name,
				preview,
				// The same warning the single-item dialog renders, under this
				// secret's own name.
				warnings: (
					<TransferReferencingWarning
						noun="secret"
						hostName={host.name}
						quadlets={preview.data?.referencingQuadlets ?? []}
						mode={transfer.mode}
					/>
				),
			};
		},
	);

	return (
		<MultiTransferDialog
			host={host}
			transfer={transfer}
			noun="secret"
			items={items}
			runItem={async (secret, context) => {
				const result = await moveSecret.mutateAsync({
					secretId: secret.name,
					targetHostId: context.targetId,
					mode: context.mode,
				});
				// A "partial" answer is a FAILURE for the batch report: the value
				// crossed but the source secret could not be removed, so the
				// secret still exists on both hosts and the row must stay
				// selected for the retry. The mapping is the shared one; the
				// sentence is this domain's.
				const partial = partialOutcome(
					result,
					"The value crossed but the source secret could not be removed.",
				);
				if (partial) return partial;
				return {
					ok: true,
					detail: transferStatusLabel(result.status),
				};
			}}
			onFinished={onFinished}
			onClose={onClose}
		/>
	);
}
