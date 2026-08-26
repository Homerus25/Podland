import { useQuery } from '@tanstack/react-query'
import { listContainers } from '@features/containers/services/containersApi'

export function useContainers(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'containers'],
    queryFn: () => listContainers(machineId),
  })
}
