import { useQuery } from "@tanstack/react-query";

// The query-side freshness factory, the twin of the list-invalidating
// mutation factory: the one owner of the freshness vocabulary. A view that
// must always show the machine's current state rides the one-shot flavor —
// zero gcTime drops the cache entry the moment the view unmounts, so a
// remount refetches instead of serving the cache. The optional enabled
// gate (defaulting to true) keeps a lazy fetch — an inspect reveal, a
// dialog-scoped snapshot — mounted but quiet until the view opens it. The
// useQuery result passes through untouched, so isPending, error, data, and
// refetch keep working at every call site.
export function useOneShotQuery<TData>(
	queryKey: readonly unknown[],
	queryFn: () => Promise<TData>,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey,
		queryFn,
		enabled: options?.enabled ?? true,
		gcTime: 0,
	});
}

// The always-refetch list flavor: a lazy tab's panel remounts on every
// activation, and refetching on mount "always" makes each re-entry issue a
// fresh request even inside the app-wide stale window — while the cached
// list stays visible (no loading flash), because the entry outlives the
// panel. Deliberately distinct from the one-shot flavor: a remounting list
// panel would flash loading under the one-shot rule.
export function useAlwaysRefetchListQuery<TData>(
	queryKey: readonly unknown[],
	queryFn: () => Promise<TData>,
) {
	return useQuery({ queryKey, queryFn, refetchOnMount: "always" });
}
