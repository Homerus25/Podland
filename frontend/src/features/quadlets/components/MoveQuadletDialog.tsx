import { Box, Checkbox, HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import type { QuadletMoveVolumeProgress } from "@features/quadlets/hooks/useQuadletMoveStream";
import { useQuadletMoveStream } from "@features/quadlets/hooks/useQuadletMoveStream";
import {
	type useMoveQuadlet,
	useQuadletMovePreview,
} from "@features/quadlets/hooks/useQuadlets";
import type {
	QuadletMovePreviewSecret,
	QuadletMovePreviewVolume,
	QuadletMoveResult,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import { buildMoveInput } from "@features/quadlets/services/quadletsApi";
import {
	TransferDialog,
	useTransferState,
} from "@shared/components/TransferDialog";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";
import { type ReactNode, useEffect, useState } from "react";

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

// The preview's kind labels: the DTO carries the backend's coarse split —
// the podman store kind ("volume" for volumes, "secret" for secrets) versus
// a host path — and the dialog spells both out through one helper, so the
// shared vocabulary lives once for the volume and the secret list alike.
function kindLabel(kind: "volume" | "secret" | "hostPath", store: string) {
	return kind === "hostPath" ? "host path" : store;
}

// The companion annotation in the volume list (issue 218): what happens to
// the companion .volume definition. "will be copied" answers the default
// transfer only — with the option unchecked the definition stays here. The
// other two states describe the target file and hold regardless of the
// option.
function companionNote(volume: QuadletMovePreviewVolume, transfer: boolean) {
	switch (volume.targetState) {
		case "identical":
			return "already present on target";
		case "differs":
			return "differs on target";
		default:
			return transfer
				? "definition will be copied"
				: "definition will not be transferred";
	}
}

// The data facts of one volume row (issue 214): the resolved podman volume
// with its source size, the target state with the target's size, and — for
// an entry migration cannot carry — the honest reason. Host paths claim
// nothing beyond their reason: no podman volume resolves, so there is no
// target state to speak of.
function VolumeDataNotes({ volume }: { volume: QuadletMovePreviewVolume }) {
	if (volume.kind !== "volume") {
		return (
			<Text color="gray.500" data-testid="move-volume-reason">
				{volume.reason}
			</Text>
		);
	}
	return (
		<>
			{volume.podmanVolume ? (
				<Text color="gray.500" data-testid="move-volume-resolved">
					resolves to “{volume.podmanVolume}”
					{volume.sizeBytes !== undefined
						? ` (${formatBytes(volume.sizeBytes)})`
						: ""}
				</Text>
			) : null}
			<Text
				color={volume.existsOnTarget ? "orange.500" : "gray.500"}
				data-testid="move-volume-target"
			>
				{volume.existsOnTarget
					? `already on target${volume.targetSizeBytes !== undefined ? ` (${formatBytes(volume.targetSizeBytes)})` : ""}`
					: "new on target"}
			</Text>
			{!volume.migratable ? (
				<Text color="gray.500" data-testid="move-volume-reason">
					{volume.reason}
				</Text>
			) : null}
		</>
	);
}

// One secret row's existence facts (issue 222): the stranded-dependency
// truth in the volume list's vocabulary — "already on target" is the
// warning (the value stays behind, orange), "new on target" the fresh
// shape, and a dangling reference renders honestly as missing here. A
// hostPath reference claims nothing: podman reads it off the host
// filesystem at run time, so no store ever holds it and both booleans are
// meaningless.
function SecretExistenceNotes({
	secret,
}: {
	secret: QuadletMovePreviewSecret;
}) {
	if (secret.kind !== "secret") return null;
	return (
		<>
			{!secret.existsOnSource ? (
				<Text color="gray.500" data-testid="move-secret-source">
					missing on this machine
				</Text>
			) : null}
			<Text
				color={secret.existsOnTarget ? "orange.500" : "gray.500"}
				data-testid="move-secret-target"
			>
				{secret.existsOnTarget ? "already on target" : "new on target"}
			</Text>
		</>
	);
}

// One data-phase progress line, the volume's latest phase winning.
function progressLabel(entry: QuadletMoveVolumeProgress): string {
	switch (entry.phase) {
		case "start":
			return "exporting…";
		case "bytes":
			return `${formatBytes(entry.bytes ?? 0)} exported…`;
		case "done":
			return `done (${formatBytes(entry.bytes ?? 0)})`;
		default:
			return `skipped — ${entry.reason ?? ""}`;
	}
}

// Live data-phase progress (issue 214): one line per volume, updated in
// place as its stream advances.
function MigrationProgress({
	progress,
}: {
	progress: QuadletMoveVolumeProgress[];
}) {
	return (
		<VStack
			align="flex-start"
			spacing={1}
			data-testid="move-migration-progress"
		>
			<HStack spacing={2}>
				<Spinner size="sm" />
				<Text>Migrating volume data…</Text>
			</HStack>
			{progress.map((entry) => (
				<HStack
					key={entry.podmanVolume}
					spacing={2}
					data-testid={`move-migration-volume-${entry.podmanVolume}`}
				>
					<Text fontFamily="mono">{entry.ref}</Text>
					<Text color="gray.500">{progressLabel(entry)}</Text>
				</HStack>
			))}
		</VStack>
	);
}

// The outcome-row narration shape both phases share (issue 222 review): a
// bold header line, then one row per outcome — a mono-font name and a
// colored result — keyed by testid. The phases differ only in their
// block testid, row testid prefix, entries' identity, display name, and
// result wording, so the shape lives here once and each phase's narration
// is a thin adapter.
function OutcomeNarration<Entry>({
	testid,
	rowTestidPrefix,
	label,
	entries,
	identity,
	name,
	renderOutcome,
}: {
	testid: string;
	rowTestidPrefix: string;
	label: string;
	entries: Entry[];
	identity: (entry: Entry) => string;
	name: (entry: Entry) => string;
	renderOutcome: (entry: Entry) => ReactNode;
}) {
	if (entries.length === 0) return null;
	return (
		<VStack align="flex-start" spacing={1} data-testid={testid}>
			<Text fontWeight="semibold">{label}</Text>
			{entries.map((outcome) => (
				<HStack
					key={identity(outcome)}
					spacing={2}
					data-testid={`${rowTestidPrefix}-${identity(outcome)}`}
				>
					<Text fontFamily="mono">{name(outcome)}</Text>
					{renderOutcome(outcome)}
				</HStack>
			))}
		</VStack>
	);
}

// The data phase's outcomes (issue 214): one line per referenced volume,
// migrated or skipped with its reason — rendered as a successful transfer's
// narration and under a partial message alike.
function MigrationNarration({ result }: { result: QuadletMoveResult }) {
	return (
		<OutcomeNarration
			testid="move-migration-report"
			rowTestidPrefix="move-migration-outcome"
			label="Volume data:"
			entries={result.volumeMigration?.volumes ?? []}
			identity={(outcome) => outcome.podmanVolume}
			name={(outcome) => outcome.ref}
			renderOutcome={(outcome) =>
				outcome.outcome === "migrated" ? (
					<Text color="green.500">data migrated</Text>
				) : (
					<Text color="gray.500">skipped — {outcome.reason}</Text>
				)
			}
		/>
	);
}

// The secrets phase's outcomes (issue 222): one line per referenced
// secret, transferred or skipped with its reason — rendered beside the
// volume narration wherever the transfer's outcomes are told (a stay-open
// success report and a partial result alike).
function SecretNarration({ result }: { result: QuadletMoveResult }) {
	return (
		<OutcomeNarration
			testid="move-secret-report"
			rowTestidPrefix="move-secret-outcome"
			label="Secrets:"
			entries={result.secretTransfer?.secrets ?? []}
			identity={(outcome) => outcome.name}
			name={(outcome) => outcome.name}
			renderOutcome={(outcome) =>
				outcome.outcome === "transferred" ? (
					<Text color="green.500">value transferred</Text>
				) : (
					<Text color="gray.500">skipped — {outcome.reason}</Text>
				)
			}
		/>
	);
}

// One dialog for the whole transfer (issue 212): the shared transfer-dialog
// preset carries the skeleton — the capability-filtered target select
// (source excluded), the mode radio (default Move), the target-scoped
// preview's pending/error/collision states, the stay-open report, and the
// error region. This adapter keeps the quadlet domain: the manage-quadlets
// capability and its "you can write to" picker copy, the quadlet preview
// query, the preview-driven volume and secret warnings for both modes, and
// — move mode only — the delete modal's stop/force checkboxes with its
// defaults (stop on, force off). When the preview reports companion .volume
// quadlets (issue 218), a checkbox (default on) offers carrying their
// definitions along, the volume list annotates each companion entry, and a
// differing companion blocks confirm exactly like a collision. When the
// preview reports migratable volumes (issue 214), a checkbox (default off)
// opts the transfer into carrying their data along — the heavy,
// consequential act — the volume list shows each entry's resolved volume,
// sizes, and target state, an overwrite option appears wherever the target
// already holds a volume, a hot-export caveat shows for a running source,
// and the transfer rides the NDJSON transport: per-volume progress in
// flight, the outcomes narrated on success, a 409 refusal answered with the
// overwrite hint. When the preview reports `Secret=` references (issue 222),
// the warning area lists each with its kind and source/target existence in
// both modes; a checkbox (default off) offers carrying the values along —
// offered only to a caller who is admin on the source and when at least one
// transferable secret exists, absent otherwise — and the answer's
// secretTransfer outcomes narrate in place, keeping the dialog open the way
// the migrating transport's report does. Confirm stays disabled until the
// stranded-data warning is explicitly acknowledged — in copy and move mode
// alike, because volume data never follows the file by default.
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

	// copied/moved close the dialog — the plain transport only; a migrating
	// transfer narrates its outcomes in place below, and so does a plain
	// transfer whose answer carries a secretTransfer block (issue 222): the
	// outcomes are the answer's point, and closing would drop them. A
	// partial result deliberately falls through and keeps it open on either
	// transport.
	const result = moveQuadlet.data;
	const plainSecretReport =
		result?.secretTransfer &&
		(result.status === "copied" || result.status === "moved")
			? result
			: null;
	useEffect(() => {
		if (
			(result?.status === "copied" || result?.status === "moved") &&
			!plainSecretReport
		) {
			onClose();
		}
	}, [result, plainSecretReport, onClose]);

	const volumes = preview.data?.volumes ?? [];
	// Companion .volume entries (issue 217/218): the preview marks them with
	// the source quadlet file the reference resolves to.
	const companions = volumes.filter(
		(volume): volume is QuadletMovePreviewVolume & { quadletFile: string } =>
			volume.quadletFile !== undefined,
	);
	const differsCompanions = companions.filter(
		(volume) => volume.targetState === "differs",
	);
	const differsNames = [
		...new Set(differsCompanions.map((volume) => volume.quadletFile)),
	];
	const companionNames = [
		...new Set(companions.map((volume) => volume.quadletFile)),
	];
	// Migration candidates (issue 214): the checkbox exists only when the
	// preview reports at least one reference whose data can follow.
	const migratable = volumes.some((volume) => volume.migratable);
	const anyExistsOnTarget = volumes.some((volume) => volume.existsOnTarget);
	// Secret references (issue 222): the warning list renders whatever the
	// preview scanned, both modes. The transfer checkbox exists only for a
	// caller who is admin on the SOURCE machine — the reveal gate's
	// capability, read from the per-caller permission the app already holds,
	// no new preview field — and only when the preview reports at least one
	// transferable secret (kind secret, present on the source).
	const secrets = preview.data?.secrets ?? [];
	const secretTransferOffered =
		can(machine, "reveal-secrets") &&
		secrets.some((secret) => secret.kind === "secret" && secret.existsOnSource);
	const isPartial =
		result?.status === "partial" || moveStream.result?.status === "partial";
	// A migrating transfer's completed narration: the dialog stays open on it
	// instead of closing, the stay-open-report pattern.
	const migrationReport =
		moveStream.result?.status === "copied" ||
		moveStream.result?.status === "moved"
			? moveStream.result
			: null;
	// The stream's error is already an ApiError; the plain mutation's is an
	// axios error for the shared extractor.
	const errorDisplay = moveQuadlet.error
		? getApiError(moveQuadlet.error)
		: moveStream.error;
	const partialResult = moveStream.result ?? result;

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

	// The stay-open report: a partial result, a migrating transfer's
	// outcomes, or a plain transfer's secret outcomes — each narrates in
	// place; anything else stays the form.
	const report = isPartial ? (
		<VStack align="flex-start" spacing={3} data-testid="move-partial-message">
			<Text>
				The quadlet was installed on “{targetName}” but could not be removed
				from “{machine.name}”. It currently exists on both machines and must be
				cleaned up manually.
			</Text>
			{partialResult?.error ? (
				<Text color="red.500">{partialResult.error}</Text>
			) : null}
			{partialResult ? (
				<>
					<MigrationNarration result={partialResult} />
					<SecretNarration result={partialResult} />
				</>
			) : null}
		</VStack>
	) : migrationReport ? (
		<VStack align="flex-start" spacing={4} data-testid="move-migration-success">
			<Text>
				{migrationReport.status === "moved" ? "Moved" : "Copied"} “
				{quadlet.name}” to “{targetName}”.
			</Text>
			<MigrationNarration result={migrationReport} />
			<SecretNarration result={migrationReport} />
		</VStack>
	) : plainSecretReport ? (
		<VStack align="flex-start" spacing={4} data-testid="move-secret-success">
			<Text>
				{plainSecretReport.status === "moved" ? "Moved" : "Copied"} “
				{quadlet.name}” to “{targetName}”.
			</Text>
			<SecretNarration result={plainSecretReport} />
		</VStack>
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
				<>
					{differsCompanions.length > 0 ? (
						<Text color="red.500" data-testid="move-companion-conflict-warning">
							A volume quadlet named “{differsNames.join("”, “")}” already
							exists on {targetName} with different content. Choose another
							target or remove it there first.
						</Text>
					) : null}
					{volumes.length > 0 ? (
						<Box data-testid="move-volume-warning">
							<Text>
								This quadlet references volumes. Volume data does not move with
								the file — it stays behind on {machine.name}:
							</Text>
							<VStack align="flex-start" mt={2} spacing={1}>
								{volumes.map((volume, index) => (
									// biome-ignore lint/suspicious/noArrayIndexKey: a quadlet can mount the same source twice, so source+kind is not unique; position is the row's identity
									<HStack key={index} spacing={2} data-testid="move-volume-row">
										<Text fontFamily="mono">{volume.source}</Text>
										<Text color="gray.500">
											{kindLabel(volume.kind, "podman volume")}
										</Text>
										{volume.quadletFile ? (
											<Text color="gray.500" data-testid="move-companion-note">
												{companionNote(volume, transferCompanions)}
											</Text>
										) : null}
										<VolumeDataNotes volume={volume} />
									</HStack>
								))}
							</VStack>
							<Checkbox
								isChecked={volumesAcknowledged}
								onChange={(event) =>
									setAcknowledgedFor(event.target.checked ? targetId : null)
								}
								mt={3}
								data-testid="move-volume-confirm-option"
							>
								I understand this data stays behind
							</Checkbox>
						</Box>
					) : null}
					{secrets.length > 0 ? (
						<Box data-testid="move-secret-warning">
							<Text>
								This quadlet references secrets. Secret values do not move with
								the file — they stay behind on {machine.name}:
							</Text>
							<VStack align="flex-start" mt={2} spacing={1}>
								{secrets.map((secret, index) => (
									// biome-ignore lint/suspicious/noArrayIndexKey: a quadlet can reference the same name twice, so name alone is not unique; position is the row's identity
									<HStack key={index} spacing={2} data-testid="move-secret-row">
										<Text fontFamily="mono">{secret.name}</Text>
										<Text color="gray.500">
											{kindLabel(secret.kind, "podman secret")}
										</Text>
										<SecretExistenceNotes secret={secret} />
									</HStack>
								))}
							</VStack>
						</Box>
					) : null}
				</>
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
