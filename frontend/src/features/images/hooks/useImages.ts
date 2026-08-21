import { useQuery } from '@tanstack/react-query'
import { listImages } from '@features/images/services/imagesApi'

export function useImages(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'images'],
    queryFn: () => listImages(machineId),
  })
}
