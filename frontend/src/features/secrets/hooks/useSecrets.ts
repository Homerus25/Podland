import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createSecret,
  deleteSecret,
  listSecrets,
  type CreateSecretInput,
} from '@features/secrets/services/secretsApi'

function secretsQueryKey(machineId: string) {
  return ['machines', machineId, 'secrets'] as const
}

export function useSecrets(machineId: string) {
  return useQuery({
    queryKey: secretsQueryKey(machineId),
    queryFn: () => listSecrets(machineId),
  })
}

// A create extends the machine's secret list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateSecret(machineId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateSecretInput) => createSecret(machineId, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: secretsQueryKey(machineId) })
    },
  })
}

// A delete removes the secret from the machine, so the list refetches on
// success; no optimistic updates. Errors (409 SECRET_IN_USE) surface in the
// confirmation modal that owns the mutation.
export function useDeleteSecret(machineId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (secretId: string) => deleteSecret(machineId, secretId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: secretsQueryKey(machineId) })
    },
  })
}
