import { api } from '@shared/utils/api'
import type { Machine } from '@shared/types/api'

export async function listMachines(): Promise<Machine[]> {
  const response = await api.get<Machine[]>('/machines')
  return response.data
}
