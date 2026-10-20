import { Checkbox, Text, VStack } from "@chakra-ui/react";
import {
	MigrationProgress,
	MoveQuadletReport,
	selectMoveQuadletReport,
} from "@features/quadlets/components/MoveQuadletReport";
import {
	anyVolumeExistsOnTarget,
	hasMigratableVolume,
	isSecretTransferOffered,
	MoveQuadletWarnings,
	moveCompanionNames,
	moveCompanions,
	moveDifferingCompanions,
	movePreviewSecrets,
} from "@features/quadlets/components/MoveQuadletWarnings";
import { useQuadletMoveStream } from "@features/quadlets/hooks/useQuadletMoveStream";
import {
	type useMoveQuadlet,
	useQuadletMovePreview,
} from "@features/quadlets/hooks/useQuadlets";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { buildMoveInput } from "@features/quadlets/services/quadletsApi";
import {
	TransferDialog,
	useTransferState,
} from "@shared/components/TransferDialog";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useEffect, useState } from "react";

// The one refusal that has an in-dialog answer (issue 214): the target
// volume holds data and the transfer refused. The hint names the checkbox
// that resolves it; the error's own message names the volume.
const TARGET_NOT_EMPTY_CODE = "VOLUME_MIGRATION_TARGET_NOT_EMPTY";

// The one failure the dialog can make actionable (issue 222): the secret
// phase aborted the transfer before anything was installed. The code and
// its secret-naming message surface only on the stream's terminal error
// event (the plain 5xx body collapses to the generic INTERNAL_ERROR), so
// the hint answers the stream path; it names the way out — fix and
// re-confirm, already-delivered secrets are skipped, not duplicated.
const SECRET_TRANSFER_FAILED_CODE = "SECRET_TRANSFER_FAILED";

// One dialog for the whole transfer (issue 212): the shared transfer-dialog
// preset carries the skeleton — the capability-filtered target select
// (source excluded), the mode radio (default Move), the target-scoped
// preview's pending/error/collision states, the stay-open report, and the
// error region. This adapter keeps the quadlet domain's state and wiring
// only (issue 331, architecture review 2026-10-19 candidate C6): the
// manage-quadlets capability and its "you can write to" picker copy, the
// quadlet preview query, the option checkboxes and their defaults —
// stop/force (stop on, force off), companions on, migrate/overwrite and
// secret transfer off — the target-scoped stranded-data acknowledgement
// (issue 207), the confirm gating (a differing companion blocks confirm
// exactly like a collision, so does an unacknowledged stranded-data
// warning), the payload builder call with its transport choice, and the
// error-code hints. What the transfer warns about lives in
// MoveQuadletWarnings — the pure preview projections and the warning
// lists, handed the resolved facts as props; what it narrates lives in
// MoveQuadletReport — the narrations, the live migration progress, and the
// report precedence this adapter's close-on-success effect consumes.
//
// The success behavior is mixed — a file-only copied/moved closes, while a
// "partial" result must keep the dialog open narrating the both-machines
// state and a migrating transfer narrates its data outcomes in place —
// which the MutationDialog core's unconditional close-on-success cannot
// express; the preset's stay-open report carries it. The reset invariant
// covers the caller-owned move mutation (it lives in the list, so it
// outlives the dialog): reset() on mount and unmount always starts clean,
// and the migrating transport lives in this component for the same reason
// — mount is open, unmount is close, and a close mid-run aborts the stream.
// The both-machines invalidation lives in the hooks, not here.
export function MoveQuadletDialog({
	machine,
	quadlet,
	moveQuadlet,
	onClose,
}: {
	machine: Machine;
	quadlet: QuadletSummary;
	moveQuadlet: ReturnType<typeof useMoveQuadlet>;
	onClose: () => void;
}) {
	const transfer = useTransferState(machine, "manage-quadlets");
	const { targetId, targetName, mode } = transfer;

	const [stopFirst, setStopFirst] = useState(true);
	const [force, setForce] = useState(false);
	// The volume acknowledgement (issue 207) is an answer to one target's
	// preview, so it is stored as the target id it was given to: changing
	// the target invalidates it structurally — a new target's refetched
	// volume facts demand a fresh look, with no reset effect to forget.
	const [acknowledgedFor, setAcknowledgedFor] = useState<string | null>(null);
	const volumesAcknowledged =
		acknowledgedFor !== null && acknowledgedFor === targetId;
	// Companion .volume definitions ride along by default (issue 217's
	// backend default); the checkbox only exists to opt out.
	const [transferCompanions, setTransferCompanions] = useState(true);
	// Volume data migration (issue 214): opt-in, default off.
	const [migrate, setMigrate] = useState(false);
	const [overwrite, setOverwrite] = useState(false);
	// Secret value transfer (issue 222): opt-in, default off — the heavy,
	// consequential act of moving values off the machine.
	const [transferSecrets, setTransferSecrets] = useState(false);

	// The migrating half of the transfer (issue 214): checked migration
	// switches the response to an NDJSON operation, so the same endpoint is
	// driven through the stream transport and its terminal event lands here.
	const moveStream = useQuadletMoveStream(machine.id);

	const preview = useQuadletMovePreview(
		machine.id,
		quadlet.id,
		targetId || null,
	);

	// The preview projections live in the warnings module as exported
	// functions; the adapter computes none of them inline.
	const volumes = preview.data?.volumes ?? [];
	const companions = moveCompanions(volumes);
	const differsCompanions = moveDifferingCompanions(companions);
	const companionNames = moveCompanionNames(companions);
	const migratable = hasMigratableVolume(volumes);
	const anyExistsOnTarget = anyVolumeExistsOnTarget(volumes);
	const secrets = movePreviewSecrets(preview.data);
	const secretTransferOffered = isSecretTransferOffered(machine, secrets);

	// copied/moved close the dialog — the plain transport only; a migrating
	// transfer narrates its outcomes in place below, and so does a plain
	// transfer whose answer carries a secretTransfer block (issue 222): the
	// outcomes are the answer's point, and closing would drop them. A
	// partial result deliberately falls through and keeps it open on either
	// transport. The close decision consumes the report module's precedence
	// projection, evaluated for the plain transfer's own answer.
	const result = moveQuadlet.data;
	useEffect(() => {
		if (
			(result?.status === "copied" || result?.status === "moved") &&
			selectMoveQuadletReport(result, null)?.kind !== "secret"
		) {
			onClose();
		}
	}, [result, onClose]);

	// The stream's error is already an ApiError; the plain mutation's is an
	// axios error for the shared extractor.
	const errorDisplay = moveQuadlet.error
		? getApiError(moveQuadlet.error)
		: moveStream.error;

	const confirmTransfer = () => {
		// The builder owns the whole guard set (issue 228): which flags the
		// current preview and mode still back, and whether a checked
		// migration switches the transport.
		const { input, stream } = buildMoveInput(
			{
				targetMachineId: targetId,
				companions: companions.length > 0,
				migratable,
				anyExistsOnTarget,
				secretTransferOffered,
			},
			mode,
			{
				stop: stopFirst,
				force,
				transferCompanions,
				migrate,
				overwrite,
				transferSecrets,
			},
		);
		if (stream) {
			moveStream.run(quadlet.id, input);
		} else {
			moveQuadlet.mutate({ quadletId: quadlet.id, ...input });
		}
	};

	// The stay-open report: the report module's precedence projection picks
	// what narrates — a partial result, a migrating transfer's outcomes, or
	// a plain transfer's secret outcomes; anything else stays the form.
	const selection = selectMoveQuadletReport(result, moveStream.result);
	const report = selection ? (
		<MoveQuadletReport
			selection={selection}
			quadletName={quadlet.name}
			sourceName={machine.name}
			targetName={targetName}
		/>
	) : null;

	return (
		<TransferDialog
			machine={machine}
			transfer={transfer}
			preview={preview}
			noun="quadlet"
			itemName={quadlet.name}
			noTargetsMessage="No other machine you can write to."
			isPending={moveQuadlet.isPending || moveStream.isRunning}
			// A differing companion fails the transfer with 409 (issue 217), so
			// it blocks confirm exactly like a collision; so does an
			// unacknowledged stranded-data warning.
			confirmDisabled={
				differsCompanions.length > 0 ||
				(volumes.length > 0 && !volumesAcknowledged)
			}
			error={errorDisplay}
			renderHints={(error) => (
				<>
					{error.code === TARGET_NOT_EMPTY_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-overwrite-hint">
							The target volume already holds data. Tick “Overwrite existing
							target volumes” and confirm again to merge the migrated data into
							it.
						</Text>
					) : null}
					{error.code === SECRET_TRANSFER_FAILED_CODE ? (
						<Text mt={2} color="orange.500" data-testid="move-secret-hint">
							Nothing was installed — the secret transfer runs before the
							quadlet. Resolve the cause and confirm again; secrets already
							delivered to the target are skipped, not duplicated.
						</Text>
					) : null}
				</>
			)}
			report={report}
			reset={moveQuadlet.reset}
			warnings={
				<MoveQuadletWarnings
					volumes={volumes}
					differsCompanions={differsCompanions}
					secrets={secrets}
					sourceName={machine.name}
					targetName={targetName}
					transferCompanions={transferCompanions}
					volumesAcknowledged={volumesAcknowledged}
					onAcknowledge={(acknowledged) =>
						setAcknowledgedFor(acknowledged ? targetId : null)
					}
				/>
			}
			onConfirm={confirmTransfer}
			onClose={onClose}
		>
			{migratable ? (
				<Checkbox
					isChecked={migrate}
					onChange={(event) => setMigrate(event.target.checked)}
					data-testid="move-migrate-option"
				>
					Migrate volume data to “{targetName}”
				</Checkbox>
			) : null}
			{preview.data?.running && migrate ? (
				<Text color="orange.500" data-testid="move-hot-export-warning">
					The quadlet&apos;s resource is running. Exporting data from a live
					volume is a best-effort snapshot without consistency guarantees.
				</Text>
			) : null}
			{anyExistsOnTarget ? (
				<Checkbox
					isChecked={overwrite}
					onChange={(event) => setOverwrite(event.target.checked)}
					data-testid="move-overwrite-option"
				>
					Overwrite existing target volumes (merge the migrated data)
				</Checkbox>
			) : null}
			{companions.length > 0 ? (
				<Checkbox
					isChecked={transferCompanions}
					onChange={(event) => setTransferCompanions(event.target.checked)}
					data-testid="move-companion-option"
				>
					Transfer the volume quadlet definitions ({companionNames.join(", ")})
				</Checkbox>
			) : null}
			{secretTransferOffered ? (
				<Checkbox
					isChecked={transferSecrets}
					onChange={(event) => setTransferSecrets(event.target.checked)}
					data-testid="move-transfer-secrets-option"
				>
					Transfer the secrets&apos; values to “{targetName}”
				</Checkbox>
			) : null}
			{mode === "move" ? (
				<VStack align="flex-start" spacing={2}>
					<Checkbox
						isChecked={stopFirst}
						onChange={(event) => setStopFirst(event.target.checked)}
						data-testid="move-stop-option"
					>
						Stop the unit before moving
					</Checkbox>
					<Checkbox
						isChecked={force}
						onChange={(event) => setForce(event.target.checked)}
						data-testid="move-force-option"
					>
						Force
					</Checkbox>
				</VStack>
			) : null}
			{moveStream.isRunning ? (
				<MigrationProgress progress={moveStream.progress} />
			) : null}
		</TransferDialog>
	);
}
