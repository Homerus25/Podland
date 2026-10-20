import { Box, Checkbox, HStack, Text, VStack } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import type {
	QuadletMovePreview,
	QuadletMovePreviewSecret,
	QuadletMovePreviewVolume,
} from "@features/quadlets/services/quadletsApi";
import type { Machine } from "@shared/types/api";
import { formatBytes } from "@shared/utils/format";

// The warnings half of the quadlet transfer adapter (issue 331, architecture
// review 2026-10-19 candidate C6): what a quadlet move warns about, split
// out of MoveQuadletDialog along the seam the adapter already named. The
// module owns the pure preview projections — companions, differing
// companions and their names, migratable, any-exists-on-target, secrets,
// secret-transfer-offered — as exported DOM-free functions, plus the two
// warning lists: the companion-conflict warning, the volume list (kind
// labels, companion notes, per-entry data facts, the stranded-data
// acknowledgement checkbox), and the secret list with its existence facts.
// It takes data props only — no queries, no hooks; the adapter keeps the
// preview query and hands the resolved facts over.

// A companion .volume entry (issue 217/218): the preview marks it with the
// source quadlet file the reference resolves to.
export type MoveCompanionVolume = QuadletMovePreviewVolume & {
	quadletFile: string;
};

// The preview's kind labels: the DTO carries the backend's coarse split —
// the podman store kind ("volume" for volumes, "secret" for secrets) versus
// a host path — and the dialog spells both out through one helper, so the
// shared vocabulary lives once for the volume and the secret list alike.
export function kindLabel(
	kind: "volume" | "secret" | "hostPath",
	store: string,
) {
	return kind === "hostPath" ? "host path" : store;
}

// The companion annotation in the volume list (issue 218): what happens to
// the companion .volume definition. "will be copied" answers the default
// transfer only — with the option unchecked the definition stays here. The
// other two states describe the target file and hold regardless of the
// option.
export function companionNote(
	volume: QuadletMovePreviewVolume,
	transfer: boolean,
) {
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

// Companion .volume entries (issue 217/218): the preview marks them with
// the source quadlet file the reference resolves to.
export function moveCompanions(
	volumes: QuadletMovePreviewVolume[],
): MoveCompanionVolume[] {
	return volumes.filter(
		(volume): volume is MoveCompanionVolume => volume.quadletFile !== undefined,
	);
}

// The companions whose target file differs — the ones that fail the
// transfer with 409 (issue 217) and so block confirm exactly like a
// collision.
export function moveDifferingCompanions(
	companions: MoveCompanionVolume[],
): MoveCompanionVolume[] {
	return companions.filter((volume) => volume.targetState === "differs");
}

// The unique companion file names, first-seen order — the conflict warning
// and the companion option's label both name them.
export function moveCompanionNames(companions: MoveCompanionVolume[]) {
	return [...new Set(companions.map((volume) => volume.quadletFile))];
}

// Migration candidates (issue 214): the migrate checkbox exists only when
// the preview reports at least one reference whose data can follow.
export function hasMigratableVolume(volumes: QuadletMovePreviewVolume[]) {
	return volumes.some((volume) => volume.migratable);
}

// The overwrite option appears wherever the target already holds a volume.
export function anyVolumeExistsOnTarget(volumes: QuadletMovePreviewVolume[]) {
	return volumes.some((volume) => volume.existsOnTarget);
}

// The secret references (issue 222): the warning list renders whatever the
// preview scanned, both modes.
export function movePreviewSecrets(
	preview: QuadletMovePreview | undefined,
): QuadletMovePreviewSecret[] {
	return preview?.secrets ?? [];
}

// The transfer checkbox exists only for a caller who is admin on the SOURCE
// machine — the reveal gate's capability, read from the per-caller
// permission the app already holds, no new preview field — and only when
// the preview reports at least one transferable secret (kind secret,
// present on the source).
export function isSecretTransferOffered(
	machine: Machine,
	secrets: QuadletMovePreviewSecret[],
) {
	return (
		can(machine, "reveal-secrets") &&
		secrets.some((secret) => secret.kind === "secret" && secret.existsOnSource)
	);
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

// The warnings slot of the transfer dialog: the companion-conflict warning,
// the volume list with its stranded-data acknowledgement, and the secret
// list. The acknowledgement state stays in the adapter — an answer to one
// target's preview — and is handed over as props, together with the
// companion projections the adapter already computed for its own gating:
// the module renders, it does not re-derive.
export function MoveQuadletWarnings({
	volumes,
	differsCompanions,
	secrets,
	sourceName,
	targetName,
	transferCompanions,
	volumesAcknowledged,
	onAcknowledge,
}: {
	volumes: QuadletMovePreviewVolume[];
	// The differing-companion projection, computed by the adapter through
	// this module's exported functions — one derivation, shared by the
	// adapter's confirm gating and the conflict warning here.
	differsCompanions: MoveCompanionVolume[];
	secrets: QuadletMovePreviewSecret[];
	// The source machine's name, in the stranded-data copy.
	sourceName: string;
	// The selected target's name, in the companion-conflict copy.
	targetName: string;
	// Companion .volume definitions ride along by default (issue 217's
	// backend default); the checkbox only exists to opt out — the note in
	// the volume list answers this flag.
	transferCompanions: boolean;
	// The stranded-data acknowledgement (issue 207), owned by the adapter.
	volumesAcknowledged: boolean;
	onAcknowledge: (acknowledged: boolean) => void;
}) {
	const differsNames = moveCompanionNames(differsCompanions);
	return (
		<>
			{differsCompanions.length > 0 ? (
				<Text color="red.500" data-testid="move-companion-conflict-warning">
					A volume quadlet named “{differsNames.join("”, “")}” already exists on{" "}
					{targetName} with different content. Choose another target or remove
					it there first.
				</Text>
			) : null}
			{volumes.length > 0 ? (
				<Box data-testid="move-volume-warning">
					<Text>
						This quadlet references volumes. Volume data does not move with the
						file — it stays behind on {sourceName}:
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
						onChange={(event) => onAcknowledge(event.target.checked)}
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
						This quadlet references secrets. Secret values do not move with the
						file — they stay behind on {sourceName}:
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
	);
}
