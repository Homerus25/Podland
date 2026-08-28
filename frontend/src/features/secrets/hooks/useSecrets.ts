import { useQuery } from '@tanstack/react-query'
import { listSecrets } from '@features/secrets/services/secretsApi'

function secretsQueryKey(machineId: string) {
  return ['machines', machineId, 'secrets'] as const
}

export function useSecrets(machineId: string) {
  return useQuery({
    queryKey: secretsQueryKey(machineId),
    queryFn: () => listSecrets(machineId),
  })
}
