import type { UseMutateFunction } from '@tanstack/react-query'

// Bridge a react-query mutation into the Promise-returning shape form
// components expect: resolves when the mutation settles, so the form's
// submit state spans the whole request. Runs onSuccess when the mutation
// succeeds (typically closing the modal).
export function mutationToPromise<TData, TError, TVariables>(
  mutate: UseMutateFunction<TData, TError, TVariables>,
  variables: TVariables,
  options?: { onSuccess?: () => void },
): Promise<void> {
  return new Promise<void>((resolve) =>
    mutate(variables, {
      onSuccess: () => options?.onSuccess?.(),
      onSettled: () => resolve(),
    }),
  )
}
