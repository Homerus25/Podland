import { api } from '@shared/utils/api'

// Image as returned by GET /api/machines/{id}/images
// (backend/internal/domain/images/dto.go ImageListResponse).
export interface ImageSummary {
  id: string
  repoTags: string[]
  repoDigests: string[]
  parentID?: string
  createdAt: string
  size: number
  os?: string
  architecture?: string
  labels?: Record<string, string>
}

export async function listImages(machineId: string): Promise<ImageSummary[]> {
  const response = await api.get<ImageSummary[]>(`/machines/${machineId}/images`)
  return response.data
}
