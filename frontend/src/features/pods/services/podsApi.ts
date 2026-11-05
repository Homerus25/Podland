import { createLogSource, type LogSource } from "@shared/streaming/logSource";
import { api, hostApiPath, hostApiUrl } from "@shared/utils/api";
import { keyValueRowsToRecord } from "@shared/utils/keyValueRows";

// Pod as returned by GET /api/hosts/{id}/pods
// (backend/internal/domain/pods/dto.go PodListResponse).
export interface PodSummary {
	id: string;
	name: string;
	// Podman's pod status: Running, Exited, Created, Paused, Degraded, Stopped
	state: string;
	// RFC3339 creation timestamp
	createdAt: string;
	// Containers in the pod, infra container included
	containerCount: number;
	// Network names on the pod's infra container (omitted when empty)
	networks?: string[];
	labels?: Record<string, string>;
}

// Envelope of GET /api/hosts/{id}/pods
// (backend/internal/domain/pods/dto.go PodListResult), mirroring the
// containers list envelope.
interface PodListResult {
	pods: PodSummary[];
	total: number;
	timestamp: string;
}

export async function listPods(hostId: string): Promise<PodSummary[]> {
	const response = await api.get<PodListResult>(hostApiPath(hostId, "pods"));
	return response.data.pods;
}

// One container in the pod detail DTO
// (backend/internal/domain/pods/dto.go PodContainerRef).
export interface PodContainerRef {
	id: string;
	name: string;
	state: string;
}

// Response of GET /api/hosts/{id}/pods/{podId}
// (backend/internal/domain/pods/dto.go PodDetailResponse): the fields the
// pod detail page renders, mapped from podman's inspect report.
export interface PodDetail {
	id: string;
	name: string;
	state: string;
	createdAt: string;
	labels?: Record<string, string>;
	restartPolicy?: string;
	// Mounts as --mount-style strings ("type=…,source=…,destination=…"),
	// named volumes and binds alike
	mounts?: string[];
	containers: PodContainerRef[];
	networks?: string[];
}

export async function getPod(
	hostId: string,
	podId: string,
): Promise<PodDetail> {
	const response = await api.get<PodDetail>(hostApiPath(hostId, "pods", podId));
	return response.data;
}

// GET /api/hosts/{id}/pods/{podId}/inspect -> 200 with the raw podman
// inspect report as the top-level body. The report is deliberately untyped
// on the client: no DTO is invented for it.
export async function inspectPod(
	hostId: string,
	podId: string,
): Promise<Record<string, unknown>> {
	const response = await api.get<Record<string, unknown>>(
		hostApiPath(hostId, "pods", podId, "inspect"),
	);
	return response.data;
}

// A named volume to mount into the pod — the same shape as the container
// create request's volume entry (backend PodCreateRequest.VolumeRequest).
export interface PodVolumeRow {
	name: string;
	destination: string;
	// "ro" or "rw" (the default); empty string is the unchosen form value
	mode: string;
}

// A bind mount into the pod — the same shape as the container create
// request's mount entry (backend PodCreateRequest.MountRequest).
export interface PodMountRow {
	source: string;
	destination: string;
	mode: string;
	selinuxShared?: boolean;
	selinuxPrivate?: boolean;
	chownUser?: boolean;
}

// One row of the create-pod dialog's labels editor.
export interface PodLabelRow {
	key: string;
	value: string;
}

// One row of the create-pod dialog's port-mappings editor — the same
// shape as the container create form's port-binding row.
export interface PodPortRow {
	hostPort: string;
	containerPort: string;
	protocol: "tcp" | "udp";
}

// The create-pod dialog's form values: the form's own shape, owned here so
// the payload builder below is the single owner of the form-to-payload
// mapping and the dialog imports it (the buildCreateContainerInput
// precedent, candidate C3).
export interface PodFormValues {
	name: string;
	networks: string[];
	volumes: PodVolumeRow[];
	mounts: PodMountRow[];
	labels: PodLabelRow[];
	// Host ports to publish on the pod's infra container.
	portMappings: PodPortRow[];
	// Both limits travel as strings — the form fields are text inputs;
	// the builder parses cpus and trims memory.
	cpus: string;
	memory: string;
}

// Body of POST /api/hosts/{id}/pods
// (backend/internal/domain/pods/dto.go PodCreateRequest).
export interface CreatePodInput {
	name: string;
	networks?: string[];
	volumes?: {
		name: string;
		destination: string;
		mode?: string;
	}[];
	mounts?: {
		source: string;
		destination: string;
		mode?: string;
		selinuxShared?: boolean;
		selinuxPrivate?: boolean;
		chownUser?: boolean;
	}[];
	labels?: Record<string, string>;
	// Host ports published on the pod's infra container (backend
	// PodCreateRequest.PortMappings); the protocol defaults to tcp
	// server-side when omitted.
	portMappings?: {
		hostPort: number;
		containerPort: number;
		protocol?: "tcp" | "udp";
	}[];
	cpus?: number;
	memory?: string;
}

// A volume row is complete only when its name and destination are filled;
// empty rows are filtered out of the payload and the mode defaults to rw.
function volumeRowsToInput(rows: PodVolumeRow[]): CreatePodInput["volumes"] {
	const entries = rows.flatMap((row) => {
		const name = row.name.trim();
		const destination = row.destination.trim();
		if (!name || !destination) return [];
		return [{ name, destination, mode: row.mode || "rw" }];
	});
	return entries.length > 0 ? entries : undefined;
}

// A mount row is complete only when its source and destination are filled;
// empty rows are filtered out of the payload and the mode defaults to rw.
function mountRowsToInput(rows: PodMountRow[]): CreatePodInput["mounts"] {
	const entries = rows.flatMap((row) => {
		const source = row.source.trim();
		const destination = row.destination.trim();
		if (!source || !destination) return [];
		return [
			{
				source,
				destination,
				mode: row.mode || "rw",
				...(row.selinuxShared ? { selinuxShared: true } : {}),
				...(row.selinuxPrivate ? { selinuxPrivate: true } : {}),
				...(row.chownUser ? { chownUser: true } : {}),
			},
		];
	});
	return entries.length > 0 ? entries : undefined;
}

// A port-mapping row is complete only when both ports parse to integers;
// empty or non-numeric rows are filtered out of the payload — the same
// silent filtering as the container create page's port bindings.
function parsePort(text: string): number | undefined {
	const trimmed = text.trim();
	if (trimmed === "") return undefined;
	const port = Number(trimmed);
	return Number.isInteger(port) ? port : undefined;
}

function portRowsToInput(rows: PodPortRow[]): CreatePodInput["portMappings"] {
	const mappings = rows.flatMap((row) => {
		const hostPort = parsePort(row.hostPort);
		const containerPort = parsePort(row.containerPort);
		if (hostPort === undefined || containerPort === undefined) return [];
		return [{ hostPort, containerPort, protocol: row.protocol || "tcp" }];
	});
	return mappings.length > 0 ? mappings : undefined;
}

// Builds the POST /api/hosts/{id}/pods body from the create-pod
// dialog's form values: the single owner of the form-to-payload mapping
// (the buildCreateContainerInput precedent, candidate C3) — name trim,
// complete-row filtering for volumes and mounts, labels through the
// shared rows-to-record mapper, cpus parsed to a float, memory trimmed.
// An unparseable cpus value is dropped here; the memory string travels
// verbatim (trimmed) and podman's units parser is the validator — an
// unparseable value surfaces as 400 VALIDATION_FAILED.
export function buildCreatePodInput(values: PodFormValues): CreatePodInput {
	const cpus = Number.parseFloat(values.cpus);
	const memory = values.memory.trim();
	const volumes = volumeRowsToInput(values.volumes);
	const mounts = mountRowsToInput(values.mounts);
	const portMappings = portRowsToInput(values.portMappings ?? []);
	const labels = keyValueRowsToRecord(values.labels ?? []);

	return {
		name: values.name.trim(),
		...(values.networks.length > 0 ? { networks: values.networks } : {}),
		...(volumes ? { volumes } : {}),
		...(mounts ? { mounts } : {}),
		...(portMappings ? { portMappings } : {}),
		...(labels ? { labels } : {}),
		...(values.cpus.trim() !== "" && Number.isFinite(cpus) ? { cpus } : {}),
		...(memory !== "" ? { memory } : {}),
	};
}

// Response of POST /api/hosts/{id}/pods -> 201
// (backend/internal/domain/pods/dto.go PodCreateResponse).
export interface PodCreated {
	id: string;
	name: string;
	state: string;
	createdAt: string;
}

export async function createPod(
	hostId: string,
	input: CreatePodInput,
): Promise<PodCreated> {
	const response = await api.post<PodCreated>(
		hostApiPath(hostId, "pods"),
		input,
	);
	return response.data;
}

// The pod lifecycle actions (backend PodActionResponse): start, stop,
// pause, unpause, kill (podman's default signal, no parameter), restart.
export type PodAction =
	| "start"
	| "stop"
	| "pause"
	| "unpause"
	| "kill"
	| "restart";

export interface PodActionResult {
	id: string;
	state: string;
	timestamp: string;
}

export async function podAction(
	hostId: string,
	podId: string,
	action: PodAction,
): Promise<PodActionResult> {
	const response = await api.post<PodActionResult>(
		hostApiPath(hostId, "pods", podId, action),
	);
	return response.data;
}

// Log source for GET /api/hosts/{hostId}/pods/{podId}/logs: the kit
// owns the buffered tail fetch and the follow stream
// (backend/internal/domain/pods handler pair). Podman aggregates every
// container in the pod, infra included — plain output, no prefixing.
export function podLogs(hostId: string, podId: string): LogSource {
	return createLogSource(
		hostApiPath(hostId, "pods", podId, "logs"),
		hostApiUrl(hostId, "pods", podId, "logs"),
	);
}

// DELETE /api/hosts/{id}/pods/{podId} -> 204. Running containers refuse
// the removal unless force=true is passed, mirroring the container delete.
export async function removePod(
	hostId: string,
	podId: string,
	force = false,
): Promise<void> {
	await api.delete(hostApiPath(hostId, "pods", podId), {
		params: force ? { force: "true" } : undefined,
	});
}

// One entry of the prune response
// (backend/internal/domain/pods/dto.go PodPruneResponse): the removed
// pod's id, or the per-pod error when podman could not remove it. Pods
// report no reclaimed size, so there is no size field.
export interface PodPruneEntry {
	id: string;
	error?: string;
}

// POST /api/hosts/{id}/pods/prune -> 200 with the per-pod report.
export async function prunePods(hostId: string): Promise<PodPruneEntry[]> {
	const response = await api.post<PodPruneEntry[]>(
		hostApiPath(hostId, "pods", "prune"),
	);
	return response.data;
}
