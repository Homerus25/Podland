import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { CreateQuadletInput } from '@features/quadlets/services/quadletsApi'
import { createQuadlet, listQuadlets } from '@features/quadlets/services/quadletsApi'

function quadletsQueryKey(machineId: string) {
  return ['machines', machineId, 'quadlets'] as const
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
