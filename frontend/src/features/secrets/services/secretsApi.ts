import { api } from '@shared/utils/api'

// Secret as returned by GET /api/machines/{id}/secrets
// (backend/internal/domain/secrets/dto.go SecretListResponse). The backend
// deliberately omits the value: it only ever travels in the admin-only
// reveal path (GET .../secrets/{id}?reveal=true), which this type does not
// model yet.
export interface SecretSummary {
  id: string
  name: string
  // Name of the driver that stores the secret (e.g. "file")
  driver: string
  // ISO8601 creation/update timestamps ("" when the backend reports none)
  createdAt: string
  updatedAt: string
  labels?: Record<string, string>
}

export async function listSecrets(machineId: string): Promise<SecretSummary[]> {
  const response = await api.get<SecretSummary[]>(`/machines/${machineId}/secrets`)
  return response.data
}
