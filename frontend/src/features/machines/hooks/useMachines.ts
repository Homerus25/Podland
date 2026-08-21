import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createMachine,
  deleteMachine,
  getMachine,
  grantMachineAccess,
  listMachineAccess,
  listMachines,
  revokeMachineAccess,
  updateMachine,
  type CreateMachineInput,
  type UpdateMachineInput,
} from '@features/machines/services/machinesApi'
import type { MachinePermission } from '@shared/types/api'

export function useMachines() {
  return useQuery({ queryKey: ['machines'], queryFn: listMachines })
}

export function useMachine(id: string) {
  return useQuery({ queryKey: ['machines', id], queryFn: () => getMachine(id) })
}

export function useUpdateMachine(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: UpdateMachineInput) => updateMachine(id, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })
}

export function useDeleteMachine() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteMachine(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })
}

export function useMachineAccess(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'access'],
    queryFn: () => listMachineAccess(machineId),
  })
}

// Sharing changes both the grant list and the current user's own permission
// on the machine, so both query families are invalidated.
function useInvalidateSharing() {
  const queryClient = useQueryClient()
  return () => {
    queryClient.invalidateQueries({ queryKey: ['machines'] })
  }
}

export function useGrantMachineAccess(machineId: string) {
  const invalidate = useInvalidateSharing()
  return useMutation({
    mutationFn: ({ userId, permission }: { userId: string; permission: MachinePermission }) =>
      grantMachineAccess(machineId, userId, permission),
    onSuccess: invalidate,
  })
}

export function useRevokeMachineAccess(machineId: string) {
  const invalidate = useInvalidateSharing()
  return useMutation({
    mutationFn: (userId: string) => revokeMachineAccess(machineId, userId),
    onSuccess: invalidate,
  })
}

export function useCreateMachine() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateMachineInput) => createMachine(input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })
}
