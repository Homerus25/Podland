// Row-scoped mutation projection: the one owner of the ritual every
// row-action list shares. A list owns one mutation instance per action (a
// plain row function cannot own hooks), so a row derives its own pending
// and error state from the mutation's `variables`: pending is true only
// for the row whose key the in-flight variables carry, and the error
// passes through only for the row whose key the variables still name —
// react-query keeps `variables` after settle, which is what makes the
// error scoping work. Every other row sees false/null and stays usable.
// The key extractor is required and explicit at every call site (an
// identity for string-id variables, a field accessor for object
// variables); no shape-sniffing, no default extractor. Pure by design:
// no hooks, no DOM, no QueryClient. Settled 2026-11-06 (architecture
// review, candidate 2); see the React Query section of
// docs/frontend-conventions.md.

export type RowScopedMutation<TVariables, TError = Error> = {
	isPending: boolean;
	error: TError | null;
	variables: TVariables | undefined;
};

export function rowScoped<TVariables, TError = Error>(
	mutation: RowScopedMutation<TVariables, TError>,
	rowKey: string,
	keyOf: (variables: TVariables) => string,
): { isPending: boolean; error: TError | null } {
	const mine =
		mutation.variables !== undefined && keyOf(mutation.variables) === rowKey;
	return {
		isPending: mutation.isPending && mine,
		error: mine ? mutation.error : null,
	};
}

// identityKey is the one shared identity extractor for mutations whose
// variables are the row's own string id (the containers and pods
// lifecycle actions, the hosts share section's revoke) — the identical
// per-feature copies collapsed into this export. The settlement still
// requires the extractor argument explicitly at every rowScoped call
// site; this export only removes the duplication, never the argument.
export const identityKey = (id: string) => id;
