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

// Body of POST /api/machines/{id}/secrets
// (backend/internal/domain/secrets/dto.go SecretCreateRequest). The backend
// trims the name; the value must arrive byte-identical — meaningful
// whitespace is the whole point of the no-trim rule.
export interface CreateSecretInput {
  name: string
  value: string
}

// Response of POST /api/machines/{id}/secrets -> 201
// (backend SecretCreateResponse: entities.SecretCreateReport's ID).
export interface SecretCreated {
  id: string
}

export async function createSecret(
  machineId: string,
  input: CreateSecretInput,
): Promise<SecretCreated> {
  const response = await api.post<SecretCreated>(`/machines/${machineId}/secrets`, input)
  return response.data
}

// DELETE /api/machines/{id}/secrets/{secretId} -> 204. The list DTO's name
// is the natural row key, so the secret is addressed by name. The backend
// rejects deletion with 409 SECRET_IN_USE when a container references it.
export async function deleteSecret(machineId: string, secretId: string): Promise<void> {
  await api.delete(`/machines/${machineId}/secrets/${encodeURIComponent(secretId)}`)
}
