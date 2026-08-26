import { api } from '@shared/utils/api'

// Quadlet as returned by GET /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletListResponse; raw is omitted
// in list responses).
export interface QuadletSummary {
  id: string
  name: string
  path: string
  unit: string
  // Resource type (container, pod, image, volume, network)
  resource: string
  // State of the quadlet (unknown, stopped, running, error)
  state: string
  raw?: string
}

export async function listQuadlets(machineId: string): Promise<QuadletSummary[]> {
  const response = await api.get<QuadletSummary[]>(`/machines/${machineId}/quadlets`)
  return response.data
}
