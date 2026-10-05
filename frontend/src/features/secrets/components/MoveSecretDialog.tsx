import { Text, VStack } from "@chakra-ui/react";
import {
	useMoveSecret,
	useSecretMovePreview,
} from "@features/secrets/hooks/useSecrets";
import type {
	SecretMoveResult,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import {
	TransferDialog,
	useTransferState,
} from "@shared/components/TransferDialog";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";

// The execution-time refusal the dialog can make actionable (issue 239):
// a same-named secret that appeared on the target since the preview names
// the way out.
const SECRET_ALREADY_EXISTS_CODE = "SECRET_ALREADY_EXISTS";

// The terminal report of a completed transfer (issue 239): the envelope's
// status narrated in place — the dialog stays open on it, the
// stay-open-report pattern, because a "partial" result must keep the dialog
// open naming the both-machines state.
function TransferReport({
	result,
	targetName,
}: {
	result: SecretMoveResult;
	targetName: string;
}) {
	return (
		<VStack align="flex-start" spacing={2} data-testid="move-success-report">
			<Text>
				{result.status === "moved" ? "Moved" : "Copied"} “{result.name}” to “
				{targetName}”.
			</Text>
			{result.status === "partial" ? (
				<VStack
					align="flex-start"
					spacing={1}
					data-testid="move-partial-message"
				>
					<Text>
						The value crossed but the source secret could not be removed. It
						currently exists on both machines and must be cleaned up manually.
					</Text>
					{result.error ? <Text color="red.500">{result.error}</Text> : null}
				</VStack>
			) : null}
		</VStack>
	);
}

// One dialog for the whole secret transfer (issue 239): the shared
// transfer-dialog preset carries the skeleton — the capability-filtered
// target select (source excluded), the mode radio defaulting to Move, the
// target-scoped preview's pending/error/collision states, the
// referencing-quadlet warning, the stay-open report, and the error region.
// This adapter keeps the secret domain: the manage-secrets capability, the
// secret preview query, the plain-mutation transport (the endpoint answers
// one JSON envelope — secret values are small, the NDJSON transport stays a
// volume concern, so there is no progress stream), the envelope report, and
// the SECRET_ALREADY_EXISTS hint. The both-machines invalidation lives in
// the mutation hook, not here.
export function MoveSecretDialog({
	machine,
	secret,
	onClose,
}: {
	machine: Machine;
	secret: SecretSummary;
	onClose: () => void;
}) {
	const transfer = useTransferState(machine, "manage-secrets");
	const moveSecret = useMoveSecret(machine.id);
	const preview = useSecretMovePreview(
		machine.id,
		secret.name,
		transfer.targetId || null,
	);

	const report = moveSecret.data ? (
		<TransferReport result={moveSecret.data} targetName={transfer.targetName} />
	) : null;

	return (
		<TransferDialog
			machine={machine}
			transfer={transfer}
			preview={preview}
			noun="secret"
			itemName={secret.name}
			referencingQuadlets={preview.data?.referencingQuadlets ?? []}
			isPending={moveSecret.isPending}
			error={moveSecret.error ? getApiError(moveSecret.error) : null}
			renderHints={(error) =>
				error.code === SECRET_ALREADY_EXISTS_CODE ? (
					<Text mt={2} color="orange.500" data-testid="move-collision-hint">
						A secret with this name appeared on {transfer.targetName}. Choose
						another target or remove it there first.
					</Text>
				) : null
			}
			report={report}
			onConfirm={() =>
				moveSecret.mutate({
					secretId: secret.name,
					targetMachineId: transfer.targetId,
					mode: transfer.mode,
				})
			}
			onClose={onClose}
		/>
	);
}
