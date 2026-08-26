import { api } from '@shared/utils/api'

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

// POST /api/machines/{id}/containers/{containerId}/start
// (backend/internal/domain/containers/handler.go ContainerHandler.Start).
export async function startContainer(machineId: string, containerId: string): Promise<void> {
  await api.post(`/machines/${machineId}/containers/${encodeURIComponent(containerId)}/start`)
}

// POST /api/machines/{id}/containers/{containerId}/stop
// (backend/internal/domain/containers/handler.go ContainerHandler.Stop).
export async function stopContainer(machineId: string, containerId: string): Promise<void> {
  await api.post(`/machines/${machineId}/containers/${encodeURIComponent(containerId)}/stop`)
}
