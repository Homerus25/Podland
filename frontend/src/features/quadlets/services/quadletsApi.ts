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

// The resource types the backend accepts
// (QuadletCreateRequest validation: oneof container pod image volume network).
// The list endpoint can additionally report "unknown" for unrecognized file
// paths, so QuadletSummary.resource stays a plain string.
export const QUADLET_RESOURCES = ['container', 'pod', 'image', 'volume', 'network'] as const

export type QuadletResource = (typeof QUADLET_RESOURCES)[number]

// Body of POST /api/machines/{id}/quadlets
// (backend/internal/domain/quadlets/dto.go QuadletCreateRequest).
export interface CreateQuadletInput {
  name: string
  resource: QuadletResource
  content: string
}

// Response of POST /api/machines/{id}/quadlets -> 201
// (backend/internal/domain/quadlets/dto.go QuadletCreateResponse).
export interface QuadletCreated {
  id: string
  name: string
  path?: string
  unit?: string
  resource: string
  state: string
  content: string
}

export async function createQuadlet(
  machineId: string,
  input: CreateQuadletInput,
): Promise<QuadletCreated> {
  const response = await api.post<QuadletCreated>(`/machines/${machineId}/quadlets`, input)
  return response.data
}
