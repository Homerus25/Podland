import {
	Box,
	Button,
	Checkbox,
	FormControl,
	FormLabel,
	HStack,
	Radio,
	RadioGroup,
	Select,
	Spinner,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useMachines } from "@features/machines/hooks/useMachines";
import { can } from "@features/machines/permissions";
import type { VolumeMoveProgress } from "@features/volumes/hooks/useVolumeMoveStream";
import { useVolumeMoveStream } from "@features/volumes/hooks/useVolumeMoveStream";
import { useVolumeMovePreview } from "@features/volumes/hooks/useVolumes";
import type {
	VolumeMoveMode,
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";
import { useState } from "react";

// The two execution-time refusals the dialog can make actionable (issue
// 238): the in-use move refusal names the force checkbox, the collision
// names the way out. Both ride the stream's terminal error event.
const VOLUME_IN_USE_CODE = "VOLUME_IN_USE";
const VOLUME_ALREADY_EXISTS_CODE = "VOLUME_ALREADY_EXISTS";

// One data-phase progress line: the volume's latest phase winning.
function progressLabel(progress: VolumeMoveProgress): string {
	switch (progress.phase) {
		case "start":
			return "exporting…";
		case "bytes":
			return `${formatBytes(progress.bytes ?? 0)} exported…`;
		default:
			return `done (${formatBytes(progress.bytes ?? 0)})`;
	}
}

// Live data-phase progress (issue 238): one line, updated in place as the
// stream advances.
function TransferProgress({ progress }: { progress: VolumeMoveProgress }) {
	return (
		<HStack spacing={2} data-testid="move-progress">
			<Spinner size="sm" />
			<Text>Copying volume data…</Text>
			<Text color="gray.500">{progressLabel(progress)}</Text>
		</HStack>
	);
}

// The terminal report of a completed transfer (issue 238): the envelope's
// status and byte count narrated in place — the dialog stays open on it,
// the stay-open-report pattern, because the byte count is the answer's
// point and the caller may want to read it before closing.
function TransferReport({
	result,
	targetName,
}: {
	result: VolumeMoveResult;
	targetName: string;
}) {
	return (
		<VStack align="flex-start" spacing={2} data-testid="move-success-report">
			<Text>
				{result.status === "moved" ? "Moved" : "Copied"} “{result.name}” to “
				{targetName}” ({formatBytes(result.bytes)} of data).
			</Text>
			{result.status === "partial" ? (
				<VStack
					align="flex-start"
					spacing={1}
					data-testid="move-partial-message"
				>
					<Text>
						The data crossed but the source volume could not be removed. It
						currently exists on both machines and must be cleaned up manually.
					</Text>
					{result.error ? <Text color="red.500">{result.error}</Text> : null}
				</VStack>
			) : null}
		</VStack>
	);
}

// One dialog for the whole volume transfer (issue 238): a mode radio
// (default Move), a target select of the machines the caller can manage
// volumes on (source excluded), the preview-driven warnings — a collision
// blocks confirm, an in-use source offers the delete flow's force checkbox
// (move mode only), referencing quadlets render as a warning, never a
// block — and the transfer rides the NDJSON transport: progress in flight,
// the envelope narrated on success, a refusal answered with its hint.
//
// The dialog rides the plain Modal rather than the MutationDialog core, the
// MoveQuadletDialog precedent: its success behavior is mixed — the report
// narrates in place and a "partial" result must keep the dialog open
// naming the both-machines state — which the core's unconditional
// close-on-success cannot express. The reset invariant is the same: the
// caller renders conditionally, so mount is open and unmount is close, and
// a close mid-run aborts the stream. The both-machines invalidation lives
// in the stream hook, not here.
export function MoveVolumeDialog({
	machine,
	volume,
	onClose,
}: {
	machine: Machine;
	volume: VolumeSummary;
	onClose: () => void;
}) {
	const { data: machines } = useMachines();
	// Only machines the caller can manage volumes on, source excluded: the
	// backend refuses below-write targets with 403, so the picker never
	// offers one.
	const targets = (machines ?? []).filter(
		(candidate) =>
			candidate.id !== machine.id && can(candidate, "manage-volumes"),
	);
	// The select starts on the first writable machine, so the preview loads
	// immediately; empty means no writable target exists.
	const [selectedTargetId, setSelectedTargetId] = useState("");
	const targetId = selectedTargetId || targets[0]?.id || "";
	const targetName =
		targets.find((candidate) => candidate.id === targetId)?.name ?? targetId;

	const [mode, setMode] = useState<VolumeMoveMode>("move");
	const [force, setForce] = useState(false);

	const moveStream = useVolumeMoveStream(machine.id);
	const preview = useVolumeMovePreview(
		machine.id,
		volume.name,
		targetId || null,
	);

	const referencing = preview.data?.referencingQuadlets ?? [];
	// An execution-time in-use refusal (a container started between the
	// preview and the confirm) must surface the force checkbox even when
	// the stale preview says not-in-use — the refusal's hint names the
	// checkbox, so it must exist to tick.
	const inUseRefusal = moveStream.error?.code === VOLUME_IN_USE_CODE;
	const confirmDisabled =
		targetId === "" ||
		moveStream.isRunning ||
		preview.isPending ||
		preview.isError ||
		!!preview.data?.collision;

	const confirmTransfer = () => {
		moveStream.run(volume.name, {
			targetMachineId: targetId,
			mode,
			// Force is move-mode vocabulary (the delete flow's flag): copy
			// never removes anything and omits it entirely.
			...(mode === "move" ? { force } : {}),
		});
	};

	const report = moveStream.result;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={`Move or copy ${volume.name}`}
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
							isLoading={moveStream.isRunning}
							data-testid="confirm-move-button"
						>
							{mode === "move" ? "Move volume" : "Copy volume"}
						</Button>
					</>
				)
			}
		>
			{moveStream.error ? (
				<Box mt={2}>
					<ErrorDisplay
						error={moveStream.error}
						data-testid="mutation-dialog-error"
					/>
					{moveStream.error.code === VOLUME_IN_USE_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-inuse-hint">
							Force is required to move a volume that containers are using. Tick
							force and confirm again to stop them and remove the volume.
						</Text>
					) : null}
					{moveStream.error.code === VOLUME_ALREADY_EXISTS_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-collision-hint">
							A volume with this name appeared on {targetName}. Choose another
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
								No other machine you can manage volumes on.
							</Text>
						)}
					</FormControl>
					<FormControl>
						<FormLabel>Mode</FormLabel>
						<RadioGroup
							value={mode}
							onChange={(next) => setMode(next as VolumeMoveMode)}
							data-testid="move-mode-radio"
						>
							<HStack spacing={4}>
								<Radio value="move" data-testid="move-mode-move">
									Move (remove from this machine)
								</Radio>
								<Radio value="copy" data-testid="move-mode-copy">
									Copy (keep this machine&apos;s volume)
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
							A volume named “{volume.name}” already exists on {targetName}.
							Choose another target or remove it there first.
						</Text>
					) : null}
					{mode === "move" && (preview.data?.inUse || inUseRefusal) ? (
						<Box data-testid="move-inuse-warning">
							<Text color="orange.500">
								A container on this machine is using this volume. The move
								refuses without force.
							</Text>
							<Checkbox
								isChecked={force}
								onChange={(event) => setForce(event.target.checked)}
								mt={2}
								data-testid="move-force-option"
							>
								Force (stops containers using this volume before removal)
							</Checkbox>
						</Box>
					) : null}
					{referencing.length > 0 ? (
						<Box data-testid="move-referencing-warning">
							<Text>
								Quadlets on this machine reference this volume. They are not
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
									After a move these references dangle — the volume they name is
									gone from this machine.
								</Text>
							) : null}
						</Box>
					) : null}
					{moveStream.isRunning && moveStream.progress ? (
						<TransferProgress progress={moveStream.progress} />
					) : null}
				</VStack>
			)}
		</Modal>
	);
}
