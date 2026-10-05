import { Box, Checkbox, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import type { VolumeMoveProgress } from "@features/volumes/hooks/useVolumeMoveStream";
import { useVolumeMoveStream } from "@features/volumes/hooks/useVolumeMoveStream";
import { useVolumeMovePreview } from "@features/volumes/hooks/useVolumes";
import type {
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import {
	TransferDialog,
	useTransferState,
} from "@shared/components/TransferDialog";
import type { Machine } from "@shared/types/api";
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

// One dialog for the whole volume transfer (issue 238): the shared
// transfer-dialog preset carries the skeleton — the capability-filtered
// target select (source excluded), the mode radio defaulting to Move, the
// target-scoped preview's pending/error/collision states, the
// referencing-quadlet warning, the stay-open report, and the error region.
// This adapter keeps the volume domain: the manage-volumes capability, the
// volume preview query, the NDJSON transport (progress in flight, the
// envelope narrated on success), the in-use warning with the delete flow's
// force checkbox (move mode only), and the two refusal hints. A close
// mid-run aborts the stream; the both-machines invalidation lives in the
// stream hook, not here.
export function MoveVolumeDialog({
	machine,
	volume,
	onClose,
}: {
	machine: Machine;
	volume: VolumeSummary;
	onClose: () => void;
}) {
	const transfer = useTransferState(machine, "manage-volumes");
	const moveStream = useVolumeMoveStream(machine.id);
	const preview = useVolumeMovePreview(
		machine.id,
		volume.name,
		transfer.targetId || null,
	);
	const [force, setForce] = useState(false);

	// An execution-time in-use refusal (a container started between the
	// preview and the confirm) must surface the force checkbox even when
	// the stale preview says not-in-use — the refusal's hint names the
	// checkbox, so it must exist to tick.
	const inUseRefusal = moveStream.error?.code === VOLUME_IN_USE_CODE;
	const report = moveStream.result ? (
		<TransferReport
			result={moveStream.result}
			targetName={transfer.targetName}
		/>
	) : null;

	return (
		<TransferDialog
			machine={machine}
			transfer={transfer}
			preview={preview}
			noun="volume"
			itemName={volume.name}
			referencingQuadlets={preview.data?.referencingQuadlets ?? []}
			isPending={moveStream.isRunning}
			error={moveStream.error}
			renderHints={(error) => (
				<>
					{error.code === VOLUME_IN_USE_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-inuse-hint">
							Force is required to move a volume that containers are using. Tick
							force and confirm again to stop them and remove the volume.
						</Text>
					) : null}
					{error.code === VOLUME_ALREADY_EXISTS_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-collision-hint">
							A volume with this name appeared on {transfer.targetName}. Choose
							another target or remove it there first.
						</Text>
					) : null}
				</>
			)}
			report={report}
			warnings={
				transfer.mode === "move" && (preview.data?.inUse || inUseRefusal) ? (
					<Box data-testid="move-inuse-warning">
						<Text color="orange.500">
							A container on this machine is using this volume. The move refuses
							without force.
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
				) : null
			}
			onConfirm={() =>
				moveStream.run(volume.name, {
					targetMachineId: transfer.targetId,
					mode: transfer.mode,
					// Force is move-mode vocabulary (the delete flow's flag): copy
					// never removes anything and omits it entirely.
					...(transfer.mode === "move" ? { force } : {}),
				})
			}
			onClose={onClose}
		>
			{moveStream.isRunning && moveStream.progress ? (
				<TransferProgress progress={moveStream.progress} />
			) : null}
		</TransferDialog>
	);
}
