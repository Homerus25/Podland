import { createLogSource, type LogSource } from "@shared/streaming/logSource";
import { api, machineApiPath, machineApiUrl } from "@shared/utils/api";

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

// Body of POST /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerCreateRequest; only the
// fields the create form exposes — everything else is optional server-side).
export interface CreateContainerInput {
	image: string;
	name?: string;
	command?: string[];
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

// DELETE /api/machines/{id}/containers/{containerId} -> 204
// (backend/internal/domain/containers/handler.go ContainerHandler.Delete).
// The force query param exists on the endpoint but is out of scope for now.
export async function deleteContainer(
	machineId: string,
	containerId: string,
): Promise<void> {
	await api.delete(machineApiPath(machineId, "containers", containerId));
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
