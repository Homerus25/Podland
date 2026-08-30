import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import {
  createMachine,
  deleteMachine,
  getMachine,
  getMachineStatus,
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

// Per-machine live probe. staleTime 0 and no refetch interval: status is
// fresh on every mount of the overview and never polled (no-polling
// precedent). Query key includes the machine id so each card probes
// independently and the machines list is never blocked.
export function useMachineStatus(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'status'],
    queryFn: () => getMachineStatus(machineId),
    staleTime: 0,
  })
}

// The backend populates the caller's `permission` only on the list endpoint
// (backend/internal/domain/machines/service.go MachineResponse); the detail
// response omits it. So the list query is consulted to fill in the
// permission for the detail page's gating (badges, secrets, settings).
export function useMachine(id: string) {
  const detail = useQuery({ queryKey: ['machines', id], queryFn: () => getMachine(id) })
  const { data: machines } = useMachines()

  return useMemo(() => {
    const listed = machines?.find((machine) => machine.id === id)
    if (!detail.data || !listed?.permission) return detail
    return { ...detail, data: { ...detail.data, permission: listed.permission } }
  }, [detail, machines, id])
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
