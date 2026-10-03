import { createLogSource, type LogSource } from "@shared/streaming/logSource";
import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";

// Quadlet as returned by GET /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletListResponse; raw is omitted
// in list responses).
export interface QuadletSummary {
	id: string;
	name: string;
	path: string;
	unit: string;
	// Resource type (container, pod, image, volume, network)
	resource: string;
	// State of the quadlet (unknown, stopped, running, error)
	state: string;
	raw?: string;
}

export async function listQuadlets(
	machineId: string,
): Promise<QuadletSummary[]> {
	const response = await api.get<QuadletSummary[]>(
		machineApiPath(machineId, "quadlets"),
	);
	return response.data;
}

// The resource types the backend accepts
// (QuadletCreateRequest validation: oneof container pod image volume network).
// The list endpoint can additionally report "unknown" for unrecognized file
// paths, so QuadletSummary.resource stays a plain string.
export const QUADLET_RESOURCES = [
	"container",
	"pod",
	"image",
	"volume",
	"network",
] as const;

export type QuadletResource = (typeof QUADLET_RESOURCES)[number];

// Body of POST /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletCreateRequest).
export interface CreateQuadletInput {
	name: string;
	resource: QuadletResource;
	content: string;
}

// Response of POST /api/machines/{id}/quadlets -> 201
// (backend/internal/domain/quadlets/dto.go QuadletCreateResponse).
export interface QuadletCreated {
	id: string;
	name: string;
	path?: string;
	unit?: string;
	resource: string;
	state: string;
	content: string;
}

export async function createQuadlet(
	machineId: string,
	input: CreateQuadletInput,
): Promise<QuadletCreated> {
	const response = await api.post<QuadletCreated>(
		machineApiPath(machineId, "quadlets"),
		input,
	);
	return response.data;
}

// Response of GET /api/machines/{id}/quadlets/{quadletId} and of a successful
// update (backend/internal/domain/quadlets/dto.go QuadletGetResponse).
export interface QuadletDetail {
	id: string;
	name: string;
	path: string;
	unit: string;
	resource: string;
	state: string;
	content: string;
}

export async function getQuadlet(
	machineId: string,
	quadletId: string,
): Promise<QuadletDetail> {
	const response = await api.get<QuadletDetail>(
		machineApiPath(machineId, "quadlets", quadletId),
	);
	return response.data;
}

// Body of PUT /api/machines/{id}/quadlets/{quadletId}
// (backend/internal/domain/quadlets/dto.go QuadletUpdateRequest). Content
// only: the name comes from the URL path and the resource type is derived
// from the existing quadlet file path, so neither is sent.
export interface UpdateQuadletInput {
	content: string;
}

export async function updateQuadlet(
	machineId: string,
	quadletId: string,
	input: UpdateQuadletInput,
): Promise<QuadletDetail> {
	const response = await api.put<QuadletDetail>(
		machineApiPath(machineId, "quadlets", quadletId),
		input,
	);
	return response.data;
}

// DELETE /api/machines/{id}/quadlets/{quadletId} -> 204
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Delete).
// stop stops the unit before the quadlet file is removed and defaults to
// true in the backend; force enables aggressive teardown and defaults to
// false. Both are always sent so the request reflects the chosen options.
export interface DeleteQuadletOptions {
	stop: boolean;
	force: boolean;
}

export async function deleteQuadlet(
	machineId: string,
	quadletId: string,
	options: DeleteQuadletOptions,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "quadlets", quadletId), {
		params: { stop: options.stop, force: options.force },
	});
}

// POST /api/machines/{id}/quadlets/{quadletId}/start -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Start).
// Start takes no options per the backend contract.
export async function startQuadlet(
	machineId: string,
	quadletId: string,
): Promise<void> {
	await api.post(machineApiPath(machineId, "quadlets", quadletId, "start"));
}

// Query parameters for the stop and restart endpoints
// (backend/internal/domain/quadlets/handler.go parseLifecycleOptions). There
// is no force parameter (systemd ignores --force for stop/restart, issue
// 205). timeout is seconds and unset by default, in which case it is omitted
// from the request entirely so systemd's own default applies.
export interface QuadletLifecycleOptions {
	timeout?: number;
}

// POST /api/machines/{id}/quadlets/{quadletId}/stop -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Stop).
export async function stopQuadlet(
	machineId: string,
	quadletId: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	return postLifecycle(machineId, quadletId, "stop", options);
}

// POST /api/machines/{id}/quadlets/{quadletId}/restart -> 200
// (backend/internal/domain/quadlets/handler.go QuadletHandler.Restart);
// same option set as stop.
export async function restartQuadlet(
	machineId: string,
	quadletId: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	return postLifecycle(machineId, quadletId, "restart", options);
}

// Shared stop/restart body: the endpoints differ only in the path segment,
// and both take the same query parameters via lifecycleParams.
async function postLifecycle(
	machineId: string,
	quadletId: string,
	action: string,
	options: QuadletLifecycleOptions,
): Promise<void> {
	await api.post(
		machineApiPath(machineId, "quadlets", quadletId, action),
		undefined,
		{
			params: lifecycleParams(options),
		},
	);
}

function lifecycleParams(options: QuadletLifecycleOptions) {
	return options.timeout === undefined ? {} : { timeout: options.timeout };
}

// Log source for GET /api/machines/{machineId}/quadlets/{quadletId}/logs:
// the kit owns the buffered tail fetch and the follow stream
// (backend/internal/domain/quadlets handler pair).
export function quadletLogs(machineId: string, quadletId: string): LogSource {
	return createLogSource(
		machineApiPath(machineId, "quadlets", quadletId, "logs"),
		machineApiUrl(machineId, "quadlets", quadletId, "logs"),
	);
}

// Full stream URL for POST .../quadlets/auto-update; the ?dryRun=true query
// is the builder's, not the caller's (no query on a real run).
export function autoUpdateStreamUrl(
	machineId: string,
	options: { dryRun: boolean },
): string {
	const query = options.dryRun ? "?dryRun=true" : "";
	return `${machineApiUrl(machineId, "quadlets", "auto-update")}${query}`;
}

// Kind label of one previewed volume source
// (backend/internal/domain/quadlets/dto.go MovePreviewVolumeKind*): a source
// with no path separator is a named podman volume, anything else a host bind
// path. The parse is backend-owned; the frontend only labels.
export type QuadletMoveVolumeKind = "volume" | "hostPath";

// How a companion .volume quadlet file sits on the TARGET machine
// (backend/internal/domain/quadlets/dto.go MovePreviewCompanion*): absent,
// byte-identical, or with different content. An identical companion is
// skipped by the transfer; a differing one fails it with 409.
export type QuadletMoveCompanionState = "absent" | "identical" | "differs";

// One `Volume=` reference in the move/copy preview
// (backend/internal/domain/quadlets/dto.go QuadletMovePreviewVolume).
export interface QuadletMovePreviewVolume {
	source: string;
	kind: QuadletMoveVolumeKind;
	// Companion .volume quadlet fields (issue 217): present exactly when the
	// reference names a same-named quadlet file on the source machine;
	// quadletFile is that file's name and targetState labels how it sits on
	// the target. A dangling reference carries neither field.
	quadletFile?: string;
	targetState?: QuadletMoveCompanionState;
	// Volume-data fields (issue 214): podmanVolume is the podman volume the
	// reference resolves to on BOTH machines — volume-kind entries only,
	// host paths never resolve. sizeBytes/targetSizeBytes are the source
	// and target volumes' content sizes when the engine can report one
	// (podman's service API reports none today). existsOnTarget is always
	// present on volume-kind entries — false is meaningful, a fresh target.
	// migratable is always present; reason explains a non-migratable entry
	// and is present only when migratable is false.
	podmanVolume?: string;
	sizeBytes?: number;
	targetSizeBytes?: number;
	existsOnTarget: boolean;
	migratable: boolean;
	reason?: string;
}

// Kind label of one previewed secret reference
// (backend/internal/domain/quadlets/dto.go MovePreviewSecretKind*): a
// reference whose resolved source is an absolute path is read off the host
// filesystem at run time and never enters a secret store; anything else
// names a podman secret. The parse is backend-owned; the frontend only
// labels.
export type QuadletMoveSecretKind = "secret" | "hostPath";

// One `Secret=` reference in the move/copy preview (issue 215).
// existsOnSource/existsOnTarget answer whether the respective machine's
// podman secret store holds the name — meaningful on secret-kind entries
// only; a hostPath entry renders both false (no existence claims), the
// same way host-path volume entries do. A secret-kind reference with
// existsOnSource false is an honest dangling entry, not an error.
export interface QuadletMovePreviewSecret {
	name: string;
	kind: QuadletMoveSecretKind;
	existsOnSource: boolean;
	existsOnTarget: boolean;
}

// Response of GET /api/machines/{id}/quadlets/{quadletId}/move-preview
// (backend/internal/domain/quadlets/dto.go QuadletMovePreviewResponse): the
// referenced volumes and secrets in file order, whether the target already
// holds a quadlet of the same name and resource, and the source's live
// state.
export interface QuadletMovePreview {
	volumes: QuadletMovePreviewVolume[];
	// Every `Secret=` reference in the quadlet content, in file order
	// (issue 215). Always computed, flag-independent — empty (not null)
	// when the quadlet references none.
	secrets: QuadletMovePreviewSecret[];
	collision: boolean;
	running: boolean;
}

export async function quadletMovePreview(
	machineId: string,
	quadletId: string,
	targetMachineId: string,
): Promise<QuadletMovePreview> {
	const response = await api.get<QuadletMovePreview>(
		machineApiPath(machineId, "quadlets", quadletId, "move-preview"),
		{ params: { targetMachineId } },
	);
	return response.data;
}

// Mode of POST /api/machines/{id}/quadlets/{quadletId}/move
// (backend/internal/domain/quadlets/dto.go MoveMode*): "copy" installs on
// the target only; "move" additionally removes the source after the install
// succeeded.
export type QuadletMoveMode = "copy" | "move";

// Body of the move/copy transfer endpoint
// (backend/internal/domain/quadlets/dto.go QuadletMoveRequest). stop/force
// are move-mode vocabulary (the delete flow's flags): copy never stops
// anything and ignores them, so copy requests omit them entirely.
export interface QuadletMoveInput {
	targetMachineId: string;
	mode: QuadletMoveMode;
	stop?: boolean;
	force?: boolean;
	// Companion .volume quadlet files ride along by default (issue 217);
	// false transfers only the main quadlet. Sent only when the preview
	// found companions — without them the field carries no meaning, and the
	// backend's default (true) stands.
	transferVolumeQuadlets?: boolean;
	// Volume data migration (issue 214): true carries the referenced podman
	// volumes' data along AND switches the response transport to NDJSON
	// streaming. Sent only when checked — unchecked keeps the plain request.
	migrateVolumeData?: boolean;
	// Merge the imported data into target volumes that already hold content
	// (issue 214); without it such a target is refused with 409
	// VOLUME_MIGRATION_TARGET_NOT_EMPTY. Only read by the backend alongside
	// migrateVolumeData, so it is sent only then.
	overwriteVolumes?: boolean;
	// Secret value transfer (issue 215): true reveals every referenced
	// podman secret's value on the source and creates the secret on the
	// target. Requires admin on the source (the reveal gate owns value
	// exposure) and never removes a secret from the source, any mode. The
	// transport does not change — sent on the plain request, or on the
	// stream request when volume migration also runs. Sent only when the
	// admin-only checkbox is rendered and checked; the backend's default
	// (false) stands otherwise.
	transferSecrets?: boolean;
}

// The preview facts the request's flags are guarded by, computed by the
// caller (MoveQuadletDialog) from one resolved preview: which target the
// preview names, whether it found companion .volume quadlets, whether any
// reference is still migratable, whether any target volume already holds
// content, and whether the caller (admin on the source) is offered the
// secret transfer at all.
export interface QuadletMoveFacts {
	targetMachineId: string;
	companions: boolean;
	migratable: boolean;
	anyExistsOnTarget: boolean;
	secretTransferOffered: boolean;
}

// The dialog's six option answers (issue 228), assembled by the caller at
// confirm from its option state — the same single object a dialog
// restructure would collapse the six option useStates into. The builder
// owns which of these reach the wire.
export interface QuadletMoveChoices {
	stop: boolean;
	force: boolean;
	transferCompanions: boolean;
	migrate: boolean;
	overwrite: boolean;
	transferSecrets: boolean;
}

// Builds the transfer request from preview facts, mode, and choices, and
// owns the whole guard set (issue 228): stop/force are move-mode vocabulary
// (copy never sends them); transferVolumeQuadlets is sent only when the
// preview found companions; migrateVolumeData only when checked AND the
// preview still offers migration — stale checked state must not smuggle a
// heavy migration into a target the preview no longer offers it for, and a
// checked migration is also the transport switch to the NDJSON stream;
// overwriteVolumes only alongside that checked migration onto a target that
// already holds a volume; transferSecrets only when checked AND still
// offered. Everything else is omitted and the backend's defaults stand.
export function buildMoveInput(
	facts: QuadletMoveFacts,
	mode: QuadletMoveMode,
	choices: QuadletMoveChoices,
): { input: QuadletMoveInput; stream: boolean } {
	const migrateChecked = choices.migrate && facts.migratable;
	const input: QuadletMoveInput = {
		targetMachineId: facts.targetMachineId,
		mode,
		...(mode === "move" ? { stop: choices.stop, force: choices.force } : {}),
		...(facts.companions
			? { transferVolumeQuadlets: choices.transferCompanions }
			: {}),
		...(choices.transferSecrets && facts.secretTransferOffered
			? { transferSecrets: true }
			: {}),
	};
	if (migrateChecked) {
		input.migrateVolumeData = true;
		if (choices.overwrite && facts.anyExistsOnTarget) {
			input.overwriteVolumes = true;
		}
	}
	return { input, stream: migrateChecked };
}

// Status of a transfer
// (backend/internal/domain/quadlets/dto.go MoveStatus*): "copied" for a
// completed copy, "moved" for a completed move, "partial" when a move's
// target install succeeded but the source removal failed — the quadlet then
// exists on both machines.
export type QuadletMoveStatus = "copied" | "moved" | "partial";

// Outcome label of one data-phase volume result
// (backend/internal/domain/quadlets/dto.go MoveVolumeOutcome*): "migrated"
// — exported from the source, imported into the target; "skipped" — the
// reference carried no data, with reason saying why.
export type QuadletMoveVolumeOutcomeLabel = "migrated" | "skipped";

// One reference's data-phase result
// (backend/internal/domain/quadlets/dto.go QuadletMoveVolumeOutcome): ref
// is the `Volume=` source as written, podmanVolume the podman volume it
// resolved to.
export interface QuadletMoveVolumeOutcome {
	ref: string;
	podmanVolume: string;
	outcome: QuadletMoveVolumeOutcomeLabel;
	// Present only for a skipped entry (a dangling reference has no source
	// volume).
	reason?: string;
}

// The transfer envelope's additive volumeMigration block (issue 214): one
// outcome per referenced volume-kind source, in file order. Present in the
// plain envelope and in the stream's terminal complete event alike — even
// when every reference was skipped, so skipped danglings stay visible.
export interface QuadletMoveVolumeMigration {
	volumes: QuadletMoveVolumeOutcome[];
}

// Outcome label of one secret-phase result
// (backend/internal/domain/quadlets/dto.go MoveSecretOutcome*):
// "transferred" — the value was revealed on the source and the secret
// created on the target; "skipped" — the reference carried nothing, with
// reason saying why.
export type QuadletMoveSecretOutcomeLabel = "transferred" | "skipped";

// One `Secret=` reference's phase result (issue 215). Name is the resolved
// secret name; reason explains a skip (same-name on target, dangling, or
// host path).
export interface QuadletMoveSecretOutcome {
	name: string;
	outcome: QuadletMoveSecretOutcomeLabel;
	// Present only for a skipped entry.
	reason?: string;
}

// The transfer envelope's additive secretTransfer block (issue 215): one
// outcome per `Secret=` reference, in file order. Present whenever the
// request opted into transferSecrets — even when every reference was
// skipped, so skipped danglings and host paths stay visible — in the plain
// envelope and in the stream's terminal complete event alike.
export interface QuadletMoveSecretTransfer {
	secrets: QuadletMoveSecretOutcome[];
}

// Response of POST /api/machines/{id}/quadlets/{quadletId}/move
// (backend/internal/domain/quadlets/dto.go QuadletMoveResponse).
export interface QuadletMoveResult {
	id: string;
	name: string;
	resource: string;
	status: QuadletMoveStatus;
	sourceMachineId: string;
	targetMachineId: string;
	path?: string;
	// Removal-failure detail, populated only when status is "partial".
	error?: string;
	// Per-volume data-phase outcomes, present only when the request opted
	// into migrateVolumeData (issue 214).
	volumeMigration?: QuadletMoveVolumeMigration;
	// Per-reference secret-phase outcomes, present only when the request
	// opted into transferSecrets (issue 215) — beside volumeMigration, in
	// the plain envelope and the stream's terminal complete event alike.
	// No progress events and no transport change ride on it alone.
	secretTransfer?: QuadletMoveSecretTransfer;
}

export async function moveQuadlet(
	machineId: string,
	quadletId: string,
	input: QuadletMoveInput,
): Promise<QuadletMoveResult> {
	const response = await api.post<QuadletMoveResult>(
		machineApiPath(machineId, "quadlets", quadletId, "move"),
		input,
	);
	return response.data;
}

// Full stream URL for POST .../quadlets/{quadletId}/move when the request
// opts into migrateVolumeData (issue 214): the endpoint answers NDJSON
// progress events with one terminal event instead of the plain envelope.
// The transfer body travels in the request, so no query is appended.
export function moveQuadletStreamUrl(
	machineId: string,
	quadletId: string,
): string {
	return machineApiUrl(machineId, "quadlets", quadletId, "move");
}
