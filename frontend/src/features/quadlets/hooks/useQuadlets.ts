import { useQuery } from '@tanstack/react-query'
import { listQuadlets } from '@features/quadlets/services/quadletsApi'

function quadletsQueryKey(machineId: string) {
  return ['machines', machineId, 'quadlets'] as const
}

export function useQuadlets(machineId: string) {
  return useQuery({
    queryKey: quadletsQueryKey(machineId),
    queryFn: () => listQuadlets(machineId),
  })
}
