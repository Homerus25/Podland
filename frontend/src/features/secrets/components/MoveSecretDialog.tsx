import {
	Box,
	Button,
	FormControl,
	FormLabel,
	HStack,
	Radio,
	RadioGroup,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useMachines } from "@features/machines/hooks/useMachines";
import { can } from "@features/machines/permissions";
import {
	useMoveSecret,
	useSecretMovePreview,
} from "@features/secrets/hooks/useSecrets";
import type {
	SecretMoveMode,
	SecretMoveResult,
	SecretSummary,
} from "@features/secrets/services/secretsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";

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

// One dialog for the whole secret transfer (issue 239): a mode radio
// (default Move), a target select of the machines the caller can manage
// secrets on (source excluded), and the preview-driven warnings — a
// collision blocks confirm, referencing quadlets render as a warning,
// never a block. The transfer rides a plain mutation: the endpoint answers
// one JSON envelope (secret values are small, the NDJSON transport stays a
// volume concern), so there is no progress stream — the envelope narrates
// on success, a refusal is answered with its hint.
//
// The dialog rides the plain Modal rather than the MutationDialog core, the
// MoveVolumeDialog precedent: its success behavior is mixed — the report
// narrates in place and a "partial" result must keep the dialog open naming
// the both-machines state — which the core's unconditional close-on-success
// cannot express. The reset invariant is the same: the caller renders
// conditionally, so mount is open and unmount is close. The both-machines
// invalidation lives in the mutation hook, not here.
export function MoveSecretDialog({
	machine,
	secret,
	onClose,
}: {
	machine: Machine;
	secret: SecretSummary;
	onClose: () => void;
}) {
	const { data: machines } = useMachines();
	// Only machines the caller can manage secrets on, source excluded: the
	// backend refuses below-write targets with 403, so the picker never
	// offers one.
	const targets = (machines ?? []).filter(
		(candidate) =>
			candidate.id !== machine.id && can(candidate, "manage-secrets"),
	);
	// The select starts on the first writable machine, so the preview loads
	// immediately; empty means no writable target exists.
	const [selectedTargetId, setSelectedTargetId] = useState("");
	const targetId = selectedTargetId || targets[0]?.id || "";
	const targetName =
		targets.find((candidate) => candidate.id === targetId)?.name ?? targetId;

	const [mode, setMode] = useState<SecretMoveMode>("move");

	const moveSecret = useMoveSecret(machine.id);
	const preview = useSecretMovePreview(
		machine.id,
		secret.name,
		targetId || null,
	);

	const referencing = preview.data?.referencingQuadlets ?? [];
	const confirmDisabled =
		targetId === "" ||
		moveSecret.isPending ||
		preview.isPending ||
		preview.isError ||
		!!preview.data?.collision;

	const confirmTransfer = () => {
		moveSecret.mutate({
			secretId: secret.name,
			targetMachineId: targetId,
			mode,
		});
	};

	const report = moveSecret.data;
	const error = moveSecret.error ? getApiError(moveSecret.error) : null;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={`Move or copy ${secret.name}`}
			footer={
				report ? (
					<Button onClick={onClose} data-testid="close-move-report-button">
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme={mode === "move" ? "red" : "blue"}
							onClick={confirmTransfer}
							isDisabled={confirmDisabled}
							isLoading={moveSecret.isPending}
							data-testid="confirm-move-button"
						>
							{mode === "move" ? "Move secret" : "Copy secret"}
						</Button>
					</>
				)
			}
		>
			{error ? (
				<Box mt={2}>
					<ErrorDisplay error={error} data-testid="mutation-dialog-error" />
					{error.code === SECRET_ALREADY_EXISTS_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-collision-hint">
							A secret with this name appeared on {targetName}. Choose another
							target or remove it there first.
						</Text>
					) : null}
				</Box>
			) : null}
			{report ? (
				<TransferReport result={report} targetName={targetName} />
			) : (
				<VStack align="flex-start" spacing={4}>
					<FormControl isRequired>
						<FormLabel htmlFor="move-target">Target machine</FormLabel>
						{targets.length > 0 ? (
							<Select
								id="move-target"
								value={targetId}
								onChange={(event) => setSelectedTargetId(event.target.value)}
								data-testid="move-target-select"
							>
								{targets.map((candidate) => (
									<option key={candidate.id} value={candidate.id}>
										{candidate.name}
									</option>
								))}
							</Select>
						) : (
							<Text color="gray.500" data-testid="move-no-targets">
								No other machine you can manage secrets on.
							</Text>
						)}
					</FormControl>
					<FormControl>
						<FormLabel>Mode</FormLabel>
						<RadioGroup
							value={mode}
							onChange={(next) => setMode(next as SecretMoveMode)}
							data-testid="move-mode-radio"
						>
							<HStack spacing={4}>
								<Radio value="move" data-testid="move-mode-move">
									Move (remove from this machine)
								</Radio>
								<Radio value="copy" data-testid="move-mode-copy">
									Copy (keep this machine&apos;s secret)
								</Radio>
							</HStack>
						</RadioGroup>
					</FormControl>
					{preview.isPending ? (
						<Text color="gray.500" data-testid="move-preview-pending">
							Checking the target machine…
						</Text>
					) : null}
					{preview.isError ? (
						<ErrorDisplay
							error={getApiError(preview.error)}
							data-testid="move-preview-error"
						/>
					) : null}
					{preview.data?.collision ? (
						<Text color="red.500" data-testid="move-collision-warning">
							A secret named “{secret.name}” already exists on {targetName}.
							Choose another target or remove it there first.
						</Text>
					) : null}
					{referencing.length > 0 ? (
						<Box data-testid="move-referencing-warning">
							<Text>
								Quadlets on this machine reference this secret. They are not
								transferred and keep pointing at {machine.name}:
							</Text>
							<VStack align="flex-start" mt={2} spacing={1}>
								{referencing.map((quadlet) => (
									<HStack
										key={quadlet.name}
										spacing={2}
										data-testid="move-referencing-row"
									>
										<Text fontFamily="mono">{quadlet.name}</Text>
									</HStack>
								))}
							</VStack>
							{mode === "move" ? (
								<Text mt={2} color="orange.500">
									After a move these references dangle — the secret they name is
									gone from this machine.
								</Text>
							) : null}
						</Box>
					) : null}
				</VStack>
			)}
		</Modal>
	);
}
