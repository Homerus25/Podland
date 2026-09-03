import type { UseMutateFunction } from "@tanstack/react-query";

// Bridge a react-query mutation into the Promise-returning shape form
// components expect: resolves when the mutation settles, so the form's
// submit state spans the whole request. Runs onSuccess with the mutation
// result when it succeeds (typically closing the modal).
export function mutationToPromise<TData, TError, TVariables>(
	mutate: UseMutateFunction<TData, TError, TVariables>,
	variables: TVariables,
	options?: { onSuccess?: (data: TData) => void },
): Promise<void> {
	return new Promise<void>((resolve) =>
		mutate(variables, {
			onSuccess: (data) => options?.onSuccess?.(data),
			onSettled: () => resolve(),
		}),
	);
}
