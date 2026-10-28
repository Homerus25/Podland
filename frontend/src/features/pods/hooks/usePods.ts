import {
	type CreatePodInput,
	createPod,
	getPod,
	inspectPod,
	listPods,
	type PodAction,
	podAction,
	prunePods,
	removePod,
} from "@features/pods/services/podsApi";
import {
	podInspectQueryKey,
	podQueryKey,
	podsQueryKey,
} from "@shared/queries/queryKeys";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";
import { useQuery } from "@tanstack/react-query";

export function usePods(machineId: string) {
	return useQuery({
		queryKey: podsQueryKey(machineId),
		queryFn: () => listPods(machineId),
		// Lazy tabs remount the panel on every activation; refetching on
		// mount makes each activation issue a fresh request, even inside the
		// app-wide five-minute stale window.
		refetchOnMount: "always",
	});
}

// The pod detail view behind the detail page: a singular sub-resource keyed
// by the pod, gcTime: 0 so every page visit refetches the pod's current
// state.
export function usePod(machineId: string, podId: string) {
	return useQuery({
		queryKey: podQueryKey(machineId, podId),
		queryFn: () => getPod(machineId, podId),
		gcTime: 0,
	});
}

// The raw inspect report behind the detail page's "Show inspect JSON"
// reveal. gcTime: 0 drops the cache entry the moment the page unmounts, so
// every reveal refetches the current report. The detail page passes
// enabled: revealed so the report fetches lazily on the first "Show
// inspect JSON" click, not on page mount.
export function usePodInspect(
	machineId: string,
	podId: string,
	options?: { enabled?: boolean },
) {
	return useQuery({
		queryKey: podInspectQueryKey(machineId, podId),
		queryFn: () => inspectPod(machineId, podId),
		enabled: options?.enabled ?? true,
		gcTime: 0,
	});
}

// A create extends the machine's pod list, so the list refetches on
// success; no optimistic updates.
export function useCreatePod(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"pods",
		(input: CreatePodInput) => createPod(machineId, input),
	);
}

// One lifecycle action (start, stop, pause, unpause, kill, restart)
// mutates the pod's containers, so the pods list refetches on success.
// Each row owns its mutation instance, so pending state and errors stay
// scoped to that row.
export function usePodAction(machineId: string, action: PodAction) {
	return useListInvalidatingMutation(machineId, "pods", (podId: string) =>
		podAction(machineId, podId, action),
	);
}

// A remove deletes the pod from the machine, so the list refetches on
// success. Errors (409 POD_STATE_CONFLICT on a running pod without force,
// 404 POD_NOT_FOUND) surface in the confirmation modal that owns the
// mutation; the forced retry passes force=true.
export function useRemovePod(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"pods",
		({ podId, force }: { podId: string; force: boolean }) =>
			removePod(machineId, podId, force),
	);
}

// Prune removes all stopped pods; the per-pod report (removed ids, per-item
// errors) is the mutation's success payload, rendered in place by the
// shared stay-open prune-report dialog.
export function usePrunePods(machineId: string) {
	return useListInvalidatingMutation(machineId, "pods", () =>
		prunePods(machineId),
	);
}
