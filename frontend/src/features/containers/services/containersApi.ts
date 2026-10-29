import { createLogSource, type LogSource } from "@shared/streaming/logSource";
import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";
import { keyValueRowsToRecord } from "@shared/utils/keyValueRows";

// Container as returned by GET /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerListResponse).
export interface ContainerSummary {
	id: string;
	names: string[];
	image: string;
	imageId?: string;
	command?: string[];
	createdAt: string;
	state: string;
	status: string;
	restarts: number;
	exited: boolean;
	exitCode?: number;
	labels?: Record<string, string>;
	networks?: string[];
	mounts?: string[];
}

// A secret mounted in the container
// (backend/internal/domain/containers/dto.go ContainerSecretResponse).
export interface ContainerSecret {
	name: string;
	uid: number;
	gid: number;
	mode: number;
}

// A named volume mount of the container
// (backend/internal/domain/containers/dto.go ContainerVolumeResponse).
export interface ContainerVolume {
	name: string;
	source: string;
	destination: string;
	rw: boolean;
}

// A port the container exposes
// (backend/internal/domain/containers/dto.go ExposedPortResponse).
export interface ExposedPort {
	containerPort: number;
	protocol: string;
}

// A published port mapping
// (backend/internal/domain/containers/dto.go PortMappingResponse).
export interface PortMapping {
	containerPort: number;
	hostPort: number;
	protocol: string;
	hostIp?: string;
}

// Container size information
// (backend/internal/domain/containers/dto.go ContainerSizeResponse).
export interface ContainerSize {
	rootFsSize: number;
	rwSize: number;
}

// The container's current resource limits (backend dto.go
// ContainerLimitsResponse): the prefill for the edit-configuration form.
// CPUs is 0 when unset; memory is empty when unset.
export interface ContainerLimits {
	cpus: number;
	memory: string;
}

// The container's current healthcheck (backend dto.go
// ContainerHealthcheckResponse): the prefill for the edit-configuration
// form. The command is empty when the healthcheck is disabled; durations
// are Go duration strings.
export interface ContainerHealthcheck {
	command: string;
	interval?: string;
	timeout?: string;
	startPeriod?: string;
	retries?: number;
	disabled: boolean;
}

// Container as returned by GET /api/machines/{id}/containers/{containerId}
// (backend/internal/domain/containers/dto.go ContainerGetResponse, extended
// with entrypoint/secrets/volumes/exposedPorts by spec 301 ticket #302).
export interface ContainerDetail {
	id: string;
	name: string;
	state: string;
	status: string;
	image: string;
	imageId?: string;
	createdAt: string;
	command?: string[];
	args?: string[];
	entrypoint?: string[];
	secrets?: ContainerSecret[];
	volumes?: ContainerVolume[];
	exposedPorts?: ExposedPort[];
	workingDir?: string;
	environment?: Record<string, string>;
	labels?: Record<string, string>;
	ports?: PortMapping[];
	mounts?: string[];
	networks?: string[];
	restarts: number;
	exited: boolean;
	exitCode?: number;
	size?: ContainerSize;
	// Current resource limits; absent when the container carries none.
	limits?: ContainerLimits;
	// Current healthcheck; absent when the container carries none.
	healthcheck?: ContainerHealthcheck;
}

// Envelope of GET /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerListResult).
interface ContainerListResult {
	containers: ContainerSummary[];
	total: number;
	timestamp: string;
}

export async function listContainers(
	machineId: string,
): Promise<ContainerSummary[]> {
	// all=true includes stopped containers; podman lists only running ones by default.
	const response = await api.get<ContainerListResult>(
		machineApiPath(machineId, "containers"),
		{
			params: { all: true },
		},
	);
	return response.data.containers;
}

// GET /api/machines/{id}/containers/{containerId} -> 200
// (backend/internal/domain/containers/dto.go ContainerGetResponse; the
// extended DTO behind the container detail page, spec 301 ticket #306).
export async function getContainer(
	machineId: string,
	containerId: string,
): Promise<ContainerDetail> {
	const response = await api.get<ContainerDetail>(
		machineApiPath(machineId, "containers", containerId),
	);
	return response.data;
}

// Restart policy of POST /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerCreateRequest
// restartPolicy oneof): the policy podman applies to the container.
export type ContainerRestartPolicy =
	| "no"
	| "always"
	| "unless-stopped"
	| "on-failure";

// Access mode of a bind mount or named volume
// (backend/internal/domain/containers/dto.go MountRequest/VolumeRequest
// mode oneof): "ro" mounts read-only, "rw" read-write (the default).
export type MountMode = "ro" | "rw";

// Protocol of a port binding
// (backend/internal/domain/containers/dto.go PortBindingRequest
// protocol): the transport the published port speaks.
export type PortProtocol = "tcp" | "udp";

// Body of POST /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerCreateRequest; only the
// fields the create form exposes — everything else is optional server-side).
export interface CreateContainerInput {
	image: string;
	name?: string;
	command?: string[];
	user?: string;
	timezone?: string;
	restartPolicy?: ContainerRestartPolicy;
	remove?: boolean;
	environment?: Record<string, string>;
	portBindings?: {
		hostPort: number;
		containerPort: number;
		protocol?: PortProtocol;
	}[];
	mounts?: {
		source: string;
		destination: string;
		mode?: MountMode;
		// SELinux relabeling and chown flags (spec 341, ticket #343):
		// z (shared), Z (private), U (chown-to-user). Absent means
		// untouched — the backend maps them onto the spec's mount
		// options and rejects z+Z together.
		selinuxShared?: boolean;
		selinuxPrivate?: boolean;
		chownUser?: boolean;
	}[];
	volumes?: {
		name: string;
		destination: string;
		mode?: MountMode;
	}[];
	networks?: string[];
	// Pod to join (optional, name or ID — the form sends the name chosen
	// in the pod selector, spec 352/356). When set, the pod owns the
	// container's networking.
	pod?: string;
	secrets?: {
		source: string;
		target?: string;
	}[];
	devices?: {
		hostPath: string;
		containerPath: string;
	}[];
	// Run the container in privileged mode
	// (backend/internal/domain/containers/dto.go ContainerCreateRequest
	// privileged): granted exactly when the form's checkbox is checked.
	privileged?: boolean;
	// Linux capabilities to add to the container
	// (backend/internal/domain/containers/dto.go ContainerCreateRequest
	// capAdd): capability names passed through verbatim — podman is the
	// validator of the names.
	capAdd?: string[];
	// CPU limit in cores (backend ContainerCreateRequest cpus): mapped
	// onto the spec's CPU limit (quota = cpus x 100000, period 100000).
	cpus?: number;
	// Memory limit in podman units syntax, e.g. "512m" (backend
	// ContainerCreateRequest memory): parsed server-side through
	// podman's units parser — an unparseable value answers 400
	// VALIDATION_FAILED.
	memory?: string;
}

// One row of the create-container form's environment field array
// (spec 313): a key/value pair as typed in the dialog.
export interface EnvRow {
	key: string;
	value: string;
}

// One row of the create-container form's port-bindings field array.
export interface PortRow {
	hostPort: string;
	containerPort: string;
	protocol: PortProtocol;
}

// One row of the create-container form's bind-mounts field array. The
// z/Z/U flags (spec 341, ticket #348) are optional so a row built
// without them counts as untouched; the form's add button always
// appends them as false.
export interface MountRow {
	source: string;
	destination: string;
	mode: MountMode;
	selinuxShared?: boolean;
	selinuxPrivate?: boolean;
	chownUser?: boolean;
}

// One row of the create-container form's named-volumes field array.
export interface VolumeRow {
	name: string;
	destination: string;
	mode: MountMode;
}

// One row of the create-container form's devices field array.
export interface DeviceRow {
	hostPath: string;
	containerPath: string;
}

// One row of the create-container form's cap-add field array (spec 341):
// a capability name as typed in the form.
export interface CapAddRow {
	capability: string;
}

// The create-container dialog's form values (spec 313): the form's own
// shape, owned here so the payload builder below is the single owner of
// the form-to-payload mapping and the dialog imports it.
export interface CreateContainerFormValues {
	image: string;
	name: string;
	command: string;
	user: string;
	timezone: string;
	// "" is the "(none)" select option; a chosen policy is one of the
	// backend's oneof values.
	restartPolicy: ContainerRestartPolicy | "";
	remove: boolean;
	environment: EnvRow[];
	portBindings: PortRow[];
	mounts: MountRow[];
	volumes: VolumeRow[];
	// The checkbox lists select by option key: the network's name and the
	// secret's id. The builder resolves keys back to payload names.
	networks: string[];
	// "" is the "No pod" select option; a chosen pod travels as its name.
	pod: string;
	secrets: string[];
	devices: DeviceRow[];
	// The Security section (spec 341): privileged grants full device and
	// kernel access; capAdd rows grant individual capabilities.
	privileged: boolean;
	capAdd: CapAddRow[];
	// The Resources section (issue #377): both limits travel as strings —
	// the form values are text either way; the builder parses cpus and
	// trims memory, mirroring the create-pod dialog's fields.
	cpus: string;
	memory: string;
}

// The backend takes the command as a string array; the form offers one text
// input and splits it on whitespace ("redis-server --appendonly yes").
function parseCommand(text: string): string[] | undefined {
	const trimmed = text.trim();
	return trimmed ? trimmed.split(/\s+/) : undefined;
}

// A port row is complete only when both ports parse to integers; empty or
// non-numeric rows are filtered out of the payload.
function parsePort(text: string): number | undefined {
	const trimmed = text.trim();
	if (trimmed === "") return undefined;
	const port = Number(trimmed);
	return Number.isInteger(port) ? port : undefined;
}

function portBindingsToInput(
	rows: PortRow[],
): CreateContainerInput["portBindings"] {
	const bindings = rows.flatMap((row) => {
		const hostPort = parsePort(row.hostPort);
		const containerPort = parsePort(row.containerPort);
		if (hostPort === undefined || containerPort === undefined) return [];
		return [{ hostPort, containerPort, protocol: row.protocol || "tcp" }];
	});
	return bindings.length > 0 ? bindings : undefined;
}

// A mount/volume row is complete only when its primary field (the mount
// source or the volume name) and its destination are filled; empty rows
// are filtered out of the payload and the access mode defaults to rw.
// The entry shape rides through buildEntry, so bind mounts and named
// volumes share this one mapping; buildEntry also receives the whole
// row so a feature-specific entry can carry extra row fields (the bind
// mount's z/Z/U flags) without the volumes entry growing them.
function storageRowsToInput<
	Row extends { destination: string; mode: MountMode },
	Entry,
>(
	rows: Row[],
	primaryOf: (row: Row) => string,
	buildEntry: (
		primary: string,
		destination: string,
		mode: MountMode,
		row: Row,
	) => Entry,
): Entry[] | undefined {
	const entries = rows.flatMap((row) => {
		const primary = primaryOf(row).trim();
		const destination = row.destination.trim();
		if (!primary || !destination) return [];
		return [buildEntry(primary, destination, row.mode || "rw", row)];
	});
	return entries.length > 0 ? entries : undefined;
}

// A device row is complete only when both paths are filled; empty rows
// are filtered out of the payload.
function devicesToInput(rows: DeviceRow[]): CreateContainerInput["devices"] {
	const devices = rows.flatMap((row) => {
		const hostPath = row.hostPath.trim();
		const containerPath = row.containerPath.trim();
		if (!hostPath || !containerPath) return [];
		return [{ hostPath, containerPath }];
	});
	return devices.length > 0 ? devices : undefined;
}

// A cap-add row is complete only when its capability is filled; empty
// rows are filtered out of the payload.
function capAddToInput(rows: CapAddRow[]): string[] | undefined {
	const capabilities = rows.flatMap((row) => {
		const capability = row.capability.trim();
		return capability ? [capability] : [];
	});
	return capabilities.length > 0 ? capabilities : undefined;
}

// Builds the POST /api/machines/{id}/containers body from the
// create-container dialog's form values (spec 313): the single owner of
// the form-to-payload mapping — trims, the command whitespace split,
// empty-key env skipping, integer-parseable port pairs, complete
// mount/volume/device rows, protocol/mode defaults, network pass-through,
// and omit-empty semantics for every optional field — plus the resource
// limits: cpus parsed to a float, memory trimmed, mirroring the
// create-pod builder. `secrets` resolves
// the form's secret ids to payload names; it is structurally typed and
// undefined-tolerant because the dialog's option fetch may still be
// pending at submit time. An id whose secret no longer exists in the
// fetched list carries nothing and is skipped.
export function buildCreateContainerInput(
	values: CreateContainerFormValues,
	secrets?: { id: string; name: string }[],
): CreateContainerInput {
	const command = parseCommand(values.command);
	// The env rows map through the shared rows-to-record mapper — one
	// owner for the ritual (candidate C4); the builder keeps the payload.
	const environment = keyValueRowsToRecord(values.environment);
	const portBindings = portBindingsToInput(values.portBindings);
	const mounts = storageRowsToInput(
		values.mounts,
		(row) => row.source,
		(source, destination, mode, row) => ({
			source,
			destination,
			mode,
			// Only a checked flag travels; an untouched row sends none of
			// the three (spec 341, ticket #348).
			...(row.selinuxShared ? { selinuxShared: true } : {}),
			...(row.selinuxPrivate ? { selinuxPrivate: true } : {}),
			...(row.chownUser ? { chownUser: true } : {}),
		}),
	);
	const volumes = storageRowsToInput(
		values.volumes,
		(row) => row.name,
		(name, destination, mode) => ({ name, destination, mode }),
	);
	const devices = devicesToInput(values.devices);
	const capAdd = capAddToInput(values.capAdd);
	// Network keys are the network names, so the selection is the
	// payload already.
	const networks = values.networks.length > 0 ? values.networks : undefined;
	// Secret keys are the secret ids; the payload carries the name. A
	// secret mounts at /run/secrets/<name> unless a target is given;
	// the form always sends the default target.
	const secretNameById = new Map(
		(secrets ?? []).map((secret) => [secret.id, secret.name]),
	);
	const secretEntries = values.secrets.flatMap((id) => {
		const name = secretNameById.get(id);
		return name ? [{ source: name, target: `/run/secrets/${name}` }] : [];
	});
	const secretPayload = secretEntries.length > 0 ? secretEntries : undefined;
	const user = values.user.trim();
	const timezone = values.timezone.trim();
	const pod = values.pod.trim();
	// Both limits mirror the create-pod builder: cpus parses to a float
	// (an unparseable value is dropped here — the field's register
	// validation keeps it from reaching the builder), memory travels
	// verbatim (trimmed) and podman's units parser is the validator.
	const cpus = Number.parseFloat(values.cpus);
	const memory = values.memory.trim();

	return {
		image: values.image.trim(),
		...(values.name.trim() ? { name: values.name.trim() } : {}),
		...(command ? { command } : {}),
		...(user ? { user } : {}),
		...(timezone ? { timezone } : {}),
		...(values.restartPolicy ? { restartPolicy: values.restartPolicy } : {}),
		...(values.remove ? { remove: true } : {}),
		...(environment ? { environment } : {}),
		...(portBindings ? { portBindings } : {}),
		...(mounts ? { mounts } : {}),
		...(volumes ? { volumes } : {}),
		// The pod owns networking (a networks list alongside a pod is
		// podman's refusal to answer), so a chosen pod drops the network
		// selection.
		...(!pod && networks ? { networks } : {}),
		...(pod ? { pod } : {}),
		...(secretPayload ? { secrets: secretPayload } : {}),
		...(devices ? { devices } : {}),
		...(values.privileged ? { privileged: true } : {}),
		...(capAdd ? { capAdd } : {}),
		...(values.cpus.trim() !== "" && Number.isFinite(cpus) ? { cpus } : {}),
		...(memory !== "" ? { memory } : {}),
	};
}

// Response of POST /api/machines/{id}/containers -> 201
// (backend/internal/domain/containers/dto.go ContainerCreateResponse).
export interface ContainerCreated {
	id: string;
	name: string;
	state: string;
	image: string;
	imageId?: string;
	createdAt: string;
}

export async function createContainer(
	machineId: string,
	input: CreateContainerInput,
): Promise<ContainerCreated> {
	const response = await api.post<ContainerCreated>(
		machineApiPath(machineId, "containers"),
		input,
	);
	return response.data;
}

// POST /api/machines/{id}/containers/{containerId}/start
// (backend/internal/domain/containers/handler.go ContainerHandler.Start).
export async function startContainer(
	machineId: string,
	containerId: string,
): Promise<void> {
	await api.post(machineApiPath(machineId, "containers", containerId, "start"));
}

// POST /api/machines/{id}/containers/{containerId}/stop
// (backend/internal/domain/containers/handler.go ContainerHandler.Stop).
export async function stopContainer(
	machineId: string,
	containerId: string,
): Promise<void> {
	await api.post(machineApiPath(machineId, "containers", containerId, "stop"));
}

// POST /api/machines/{id}/containers/{containerId}/restart
// (backend/internal/domain/containers/handler.go ContainerHandler.Restart).
// The endpoint takes no timeout or force parameters — podman's default
// timeout applies.
export async function restartContainer(
	machineId: string,
	containerId: string,
): Promise<void> {
	await api.post(
		machineApiPath(machineId, "containers", containerId, "restart"),
	);
}

// POST /api/machines/{id}/containers/{containerId}/rename -> 200
// (backend/internal/domain/containers/handler.go ContainerHandler.Rename).
// The response is the updated container detail DTO, the same shape the
// detail GET answers. The name's shape is the server's to validate: the
// client guards only the empty and unchanged cases.
export async function renameContainer(
	machineId: string,
	containerId: string,
	name: string,
): Promise<ContainerDetail> {
	const response = await api.post<ContainerDetail>(
		machineApiPath(machineId, "containers", containerId, "rename"),
		{ name },
	);
	return response.data;
}

// The healthcheck slice of an update request (backend dto.go
// ContainerHealthcheckUpdateRequest): every field optional — absent means
// unchanged. The disable flag is mutually exclusive with every other
// healthcheck field.
export interface ContainerHealthcheckUpdate {
	cmd?: string;
	interval?: string;
	timeout?: string;
	startPeriod?: string;
	retries?: number;
	disabled?: boolean;
}

// The body of POST /api/machines/{id}/containers/{containerId}/update
// (backend dto.go ContainerUpdateRequest): patch semantics — absent
// fields mean "unchanged", and only present fields are forwarded to
// podman's container update.
export interface UpdateContainerInput {
	cpus?: number;
	memory?: string;
	healthcheck?: ContainerHealthcheckUpdate;
}

// POST /api/machines/{id}/containers/{containerId}/update -> 200
// (backend/internal/domain/containers/handler.go ContainerHandler.Update).
// The response is the updated container detail DTO, the same shape the
// detail GET answers, carrying the new limits and healthcheck.
export async function updateContainer(
	machineId: string,
	containerId: string,
	input: UpdateContainerInput,
): Promise<ContainerDetail> {
	const response = await api.post<ContainerDetail>(
		machineApiPath(machineId, "containers", containerId, "update"),
		input,
	);
	return response.data;
}

// The edit-configuration form's values (spec 412, ticket #414): the
// resource-limit slice the shared inputs own, plus the healthcheck
// fields. Every field is a string the user typed (or the prefill
// rendered); the builder below maps them onto the patch payload.
export interface ContainerConfigFormValues {
	cpus: string;
	memory: string;
	command: string;
	interval: string;
	timeout: string;
	startPeriod: string;
	retries: string;
	disabled: boolean;
}

// Maps the container detail DTO onto the edit-configuration form's
// prefill: the current limits and healthcheck the user sees before
// changing anything. Unset limits and absent healthcheck fields render
// as empty strings — the form's "unchanged" vocabulary.
export function containerConfigPrefill(
	container: ContainerDetail,
): ContainerConfigFormValues {
	return {
		cpus: container.limits?.cpus ? String(container.limits.cpus) : "",
		memory: container.limits?.memory ?? "",
		command: container.healthcheck?.command ?? "",
		interval: container.healthcheck?.interval ?? "",
		timeout: container.healthcheck?.timeout ?? "",
		startPeriod: container.healthcheck?.startPeriod ?? "",
		retries:
			container.healthcheck?.retries != null
				? String(container.healthcheck.retries)
				: "",
		disabled: container.healthcheck?.disabled ?? false,
	};
}

// Maps the edit-configuration form onto the update request (the
// create-page precedent: the form-to-payload mapping lives in the
// services module). The contract is patch semantics — only fields the
// user changed travel, empty means unchanged, and clearing a limit or a
// healthcheck field is not a supported change (podman has no clear
// semantics). A field changed when its trimmed value differs from the
// trimmed prefill — whitespace alone is no change — and a changed value
// travels trimmed. The disable toggle is exclusive: when set, the
// healthcheck carries the disable flag alone.
export function buildUpdateContainerInput(
	values: ContainerConfigFormValues,
	prefill: ContainerConfigFormValues,
): UpdateContainerInput {
	const input: UpdateContainerInput = {};
	// The one changed-check: a non-empty trimmed value that differs from
	// the trimmed prefill.
	const changed = (value: string, prefillValue: string) =>
		value.trim() !== "" && value.trim() !== prefillValue.trim();
	if (changed(values.cpus, prefill.cpus)) {
		input.cpus = Number.parseFloat(values.cpus.trim());
	}
	if (changed(values.memory, prefill.memory)) {
		input.memory = values.memory.trim();
	}
	if (values.disabled) {
		if (!prefill.disabled) {
			input.healthcheck = { disabled: true };
		}
		return input;
	}
	const healthcheck: ContainerHealthcheckUpdate = {};
	if (changed(values.command, prefill.command)) {
		healthcheck.cmd = values.command.trim();
	}
	if (changed(values.interval, prefill.interval)) {
		healthcheck.interval = values.interval.trim();
	}
	if (changed(values.timeout, prefill.timeout)) {
		healthcheck.timeout = values.timeout.trim();
	}
	if (changed(values.startPeriod, prefill.startPeriod)) {
		healthcheck.startPeriod = values.startPeriod.trim();
	}
	if (changed(values.retries, prefill.retries)) {
		healthcheck.retries = Number.parseInt(values.retries, 10);
	}
	if (Object.keys(healthcheck).length > 0) {
		input.healthcheck = healthcheck;
	}
	return input;
}

// DELETE /api/machines/{id}/containers/{containerId} -> 204
// (backend/internal/domain/containers/handler.go ContainerHandler.Delete).
// The force query param exists on the endpoint but is out of scope for now.
export async function deleteContainer(
	machineId: string,
	containerId: string,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "containers", containerId));
}

// One entry of POST /api/machines/{id}/containers/prune -> 200 (backend
// containers dto.go ContainerPruneResponse): a removed container's ID and
// reclaimed size, plus podman's per-container error when removal failed.
// Podman's container prune report carries no name, so the entry is
// ID-keyed like the pods prune report.
export interface ContainerPruneReport {
	id: string;
	size: number;
	error?: string;
}

// Prunes every stopped container on the machine. Running containers are
// never touched.
export async function pruneContainers(
	machineId: string,
): Promise<ContainerPruneReport[]> {
	const response = await api.post<ContainerPruneReport[]>(
		machineApiPath(machineId, "containers", "prune"),
	);
	return response.data;
}

// GET /api/machines/{id}/containers/{containerId}/inspect -> 200 with the
// raw podman inspect report as the top-level body. The report is deliberately
// untyped on the client: no DTO is invented for it.
export async function inspectContainer(
	machineId: string,
	containerId: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		machineApiPath(machineId, "containers", containerId, "inspect"),
	);
	return response.data;
}

// Log source for GET /api/machines/{machineId}/containers/{containerId}/logs:
// the kit owns the buffered tail fetch and the follow stream
// (backend/internal/domain/containers handler pair).
export function containerLogs(
	machineId: string,
	containerId: string,
): LogSource {
	return createLogSource(
		machineApiPath(machineId, "containers", containerId, "logs"),
		machineApiUrl(machineId, "containers", containerId, "logs"),
	);
}
