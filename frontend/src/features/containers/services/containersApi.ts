import { api } from '@shared/utils/api'
import { followLogStream, type LogEvent } from '@shared/streaming/logStream'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

// Container as returned by GET /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerListResponse).
export interface ContainerSummary {
  id: string
  names: string[]
  image: string
  imageId?: string
  command?: string[]
  createdAt: string
  state: string
  status: string
  restarts: number
  exited: boolean
  exitCode?: number
  labels?: Record<string, string>
  networks?: string[]
  mounts?: string[]
}

// Envelope of GET /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerListResult).
interface ContainerListResult {
  containers: ContainerSummary[]
  total: number
  timestamp: string
}

export async function listContainers(machineId: string): Promise<ContainerSummary[]> {
  // all=true includes stopped containers; podman lists only running ones by default.
  const response = await api.get<ContainerListResult>(`/machines/${machineId}/containers`, {
    params: { all: true },
  })
  return response.data.containers
}

// Body of POST /api/machines/{id}/containers
// (backend/internal/domain/containers/dto.go ContainerCreateRequest; only the
// fields the create form exposes — everything else is optional server-side).
export interface CreateContainerInput {
  image: string
  name?: string
  command?: string[]
}

// Response of POST /api/machines/{id}/containers -> 201
// (backend/internal/domain/containers/dto.go ContainerCreateResponse).
export interface ContainerCreated {
  id: string
  name: string
  state: string
  image: string
  imageId?: string
  createdAt: string
}

export async function createContainer(
  machineId: string,
  input: CreateContainerInput,
): Promise<ContainerCreated> {
  const response = await api.post<ContainerCreated>(`/machines/${machineId}/containers`, input)
  return response.data
}

function containerUrl(machineId: string, containerId: string): string {
  return `/machines/${machineId}/containers/${encodeURIComponent(containerId)}`
}

// POST /api/machines/{id}/containers/{containerId}/start
// (backend/internal/domain/containers/handler.go ContainerHandler.Start).
export async function startContainer(machineId: string, containerId: string): Promise<void> {
  await api.post(`${containerUrl(machineId, containerId)}/start`)
}

// POST /api/machines/{id}/containers/{containerId}/stop
// (backend/internal/domain/containers/handler.go ContainerHandler.Stop).
export async function stopContainer(machineId: string, containerId: string): Promise<void> {
  await api.post(`${containerUrl(machineId, containerId)}/stop`)
}

// DELETE /api/machines/{id}/containers/{containerId} -> 204
// (backend/internal/domain/containers/handler.go ContainerHandler.Delete).
// The force query param exists on the endpoint but is out of scope for now.
export async function deleteContainer(machineId: string, containerId: string): Promise<void> {
  await api.delete(containerUrl(machineId, containerId))
}

// GET /api/machines/{id}/containers/{containerId}/logs?tail=N (no follow)
// returns the last lines as one multiline string
// (backend/internal/domain/containers/dto.go ContainerLogsResponse).
export async function fetchContainerLogs(
  machineId: string,
  containerId: string,
  tail: number,
): Promise<string[]> {
  const response = await api.get<{ id: string; logs: string }>(
    `${containerUrl(machineId, containerId)}/logs`,
    { params: { tail } },
  )
  return response.data.logs.split('\n').filter((line) => line !== '')
}

// Same endpoint with follow=true: NDJSON stream of ContainerLogEvent values.
// Uses plain fetch via the shared stream helper (axios buffers responses).
export async function followContainerLogs(
  machineId: string,
  containerId: string,
  tail: number,
  onEvent: (event: LogEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  await followLogStream(
    `${apiBaseUrl}${containerUrl(machineId, containerId)}/logs?tail=${tail}&follow=true`,
    onEvent,
    signal,
  )
}
