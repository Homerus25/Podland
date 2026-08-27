import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  CreateQuadletInput,
  DeleteQuadletOptions,
  QuadletLifecycleOptions,
  UpdateQuadletInput,
} from '@features/quadlets/services/quadletsApi'
import {
  createQuadlet,
  deleteQuadlet,
  getQuadlet,
  listQuadlets,
  restartQuadlet,
  startQuadlet,
  stopQuadlet,
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

// Mutations invalidate the machine's quadlets query on success so the list
// refetches and shows the server's post-action state; no optimistic updates.
function useQuadletMutation<TInput>(
  machineId: string,
  run: (input: TInput) => Promise<unknown>,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: run,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: quadletsQueryKey(machineId) })
    },
  })
}

export function useCreateQuadlet(machineId: string) {
  return useQuadletMutation(machineId, (input: CreateQuadletInput) =>
    createQuadlet(machineId, input),
  )
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
  return useQuadletMutation(
    machineId,
    ({ quadletId, content }: { quadletId: string } & UpdateQuadletInput) =>
      updateQuadlet(machineId, quadletId, { content }),
  )
}

// A delete removes the unit from the machine, so the list refetches on
// success and the quadlet disappears.
export function useDeleteQuadlet(machineId: string) {
  return useQuadletMutation(
    machineId,
    ({ quadletId, ...options }: { quadletId: string } & DeleteQuadletOptions) =>
      deleteQuadlet(machineId, quadletId, options),
  )
}

// Lifecycle actions change the unit's systemd state, so the list refetches on
// success and the state badge reflects what the machine now reports.
export function useStartQuadlet(machineId: string) {
  return useQuadletMutation(machineId, (quadletId: string) =>
    startQuadlet(machineId, quadletId),
  )
}

export function useStopQuadlet(machineId: string) {
  return useQuadletMutation(
    machineId,
    ({ quadletId, ...options }: { quadletId: string } & QuadletLifecycleOptions) =>
      stopQuadlet(machineId, quadletId, options),
  )
}

export function useRestartQuadlet(machineId: string) {
  return useQuadletMutation(
    machineId,
    ({ quadletId, ...options }: { quadletId: string } & QuadletLifecycleOptions) =>
      restartQuadlet(machineId, quadletId, options),
  )
}
