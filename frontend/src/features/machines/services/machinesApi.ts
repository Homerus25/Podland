import { api } from '@shared/utils/api'
import type { Machine } from '@shared/types/api'

// CreateMachineRequest (backend/internal/domain/machines/service.go):
// the SSH private key contents are read client-side via the File API and
// sent in the request body.
export interface CreateMachineInput {
  name: string
  description?: string
  hostname: string
  port: number
  sshUsername: string
  sshPrivateKey: string
  podmanSocketPath?: string
}

export async function listMachines(): Promise<Machine[]> {
  const response = await api.get<Machine[]>('/machines')
  return response.data
}

export async function getMachine(id: string): Promise<Machine> {
  const response = await api.get<Machine>(`/machines/${id}`)
  return response.data
}

export async function createMachine(input: CreateMachineInput): Promise<Machine> {
  const response = await api.post<Machine>('/machines', input)
  return response.data
}
