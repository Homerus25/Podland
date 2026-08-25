import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteImage, listImages } from '@features/images/services/imagesApi'

export function useImages(machineId: string) {
  return useQuery({
    queryKey: ['machines', machineId, 'images'],
    queryFn: () => listImages(machineId),
  })
}

export function useDeleteImage(machineId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ imageId, force }: { imageId: string; force?: boolean }) =>
      deleteImage(machineId, imageId, force ?? false),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['machines', machineId, 'images'] })
    },
  })
}
