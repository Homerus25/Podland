import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { CreateQuadletInput, UpdateQuadletInput } from '@features/quadlets/services/quadletsApi'
import {
  createQuadlet,
  getQuadlet,
  listQuadlets,
  updateQuadlet,
} from '@features/quadlets/services/quadletsApi'

function quadletsQueryKey(machineId: string) {
  return ['machines', machineId, 'quadlets'] as const
}

function quadletQueryKey(machineId: string, quadletId: string) {
  return [...quadletsQueryKey(machineId), quadletId] as const
}

export function useQuadlets(machineId: string) {
  return useQuery({
    queryKey: quadletsQueryKey(machineId),
    queryFn: () => listQuadlets(machineId),
  })
}

// Create invalidates the machine's quadlets query on success so the list
// refetches and the new unit appears.
export function useCreateQuadlet(machineId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateQuadletInput) => createQuadlet(machineId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: quadletsQueryKey(machineId) })
    },
  })
}

export function useQuadlet(machineId: string, quadletId: string) {
  return useQuery({
    queryKey: quadletQueryKey(machineId, quadletId),
    queryFn: () => getQuadlet(machineId, quadletId),
    // The editor must open on the quadlet's current content, so the detail
    // cache is dropped as soon as the modal unmounts: reopening (e.g. after
    // a save) refetches instead of reseeding the form with the pre-edit file.
    gcTime: 0,
  })
}

// A content update can change the quadlet's systemd state, so the list
// refetches on success.
export function useUpdateQuadlet(machineId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ quadletId, content }: { quadletId: string } & UpdateQuadletInput) =>
      updateQuadlet(machineId, quadletId, { content }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: quadletsQueryKey(machineId) })
    },
  })
}
