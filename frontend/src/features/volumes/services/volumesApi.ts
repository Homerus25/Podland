import { api } from '@shared/utils/api'

// Volume as returned by GET /api/machines/{id}/volumes
// (backend/internal/domain/volumes volume DTO). size is only present when
// podman reports usage for the volume. labels/options are always present
// ({} when empty).
export interface VolumeSummary {
  name: string
  driver: string
  createdAt: string
  anonymous: boolean
  labels: Record<string, string>
  options: Record<string, string>
  mountpoint: string
  size?: number
}

export async function listVolumes(machineId: string): Promise<VolumeSummary[]> {
  const response = await api.get<VolumeSummary[]>(
    `/machines/${encodeURIComponent(machineId)}/volumes`,
  )
  return response.data
}
