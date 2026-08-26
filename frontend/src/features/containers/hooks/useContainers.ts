import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  listContainers,
  startContainer,
  stopContainer,
} from '@features/containers/services/containersApi'

function containersQueryKey(machineId: string) {
  return ['machines', machineId, 'containers'] as const
}

export function useContainers(machineId: string) {
  return useQuery({
    queryKey: containersQueryKey(machineId),
    queryFn: () => listContainers(machineId),
  })
}

// Lifecycle mutations invalidate the machine's containers query on success so
// the list refetches and shows the server's post-action state; no optimistic
// updates.
function useContainerLifecycle(
  machineId: string,
  run: (containerId: string) => Promise<void>,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: run,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: containersQueryKey(machineId) })
    },
  })
}

export function useStartContainer(machineId: string) {
  return useContainerLifecycle(machineId, (containerId) =>
    startContainer(machineId, containerId),
  )
}

export function useStopContainer(machineId: string) {
  return useContainerLifecycle(machineId, (containerId) =>
    stopContainer(machineId, containerId),
  )
}
