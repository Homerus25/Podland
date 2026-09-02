import { useQuery } from '@tanstack/react-query'
import { listVolumes } from '@features/volumes/services/volumesApi'

export function useVolumes(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'volumes'],
    queryFn: () => listVolumes(machineId),
  })
}
