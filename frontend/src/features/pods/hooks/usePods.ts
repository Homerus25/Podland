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
import {
	useAlwaysRefetchListQuery,
	useOneShotQuery,
} from "@shared/queries/useFreshQuery";
import { useListInvalidatingMutation } from "@shared/queries/useListInvalidatingMutation";

export function usePods(hostId: string) {
	return useAlwaysRefetchListQuery(podsQueryKey(hostId), () =>
		listPods(hostId),
	);
}

export function usePod(hostId: string, podId: string) {
	return useOneShotQuery(podQueryKey(hostId, podId), () =>
		getPod(hostId, podId),
	);
}

export function usePodInspect(
	hostId: string,
	podId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		podInspectQueryKey(hostId, podId),
		() => inspectPod(hostId, podId),
		options,
	);
}

// A create extends the host's pod list, so the list refetches on
// success; no optimistic updates.
export function useCreatePod(hostId: string) {
	return useListInvalidatingMutation(hostId, "pods", (input: CreatePodInput) =>
		createPod(hostId, input),
	);
}

// One lifecycle action (start, stop, pause, unpause, kill, restart)
// mutates the pod's containers, so the pods list refetches on success.
// Each row owns its mutation instance, so pending state and errors stay
// scoped to that row.
export function usePodAction(hostId: string, action: PodAction) {
	return useListInvalidatingMutation(hostId, "pods", (podId: string) =>
		podAction(hostId, podId, action),
	);
}

// A remove deletes the pod from the host, so the list refetches on
// success. Errors (409 POD_STATE_CONFLICT on a running pod without force,
// 404 POD_NOT_FOUND) surface in the confirmation modal that owns the
// mutation; the forced retry passes force=true.
export function useRemovePod(hostId: string) {
	return useListInvalidatingMutation(
		hostId,
		"pods",
		({ podId, force }: { podId: string; force: boolean }) =>
			removePod(hostId, podId, force),
	);
}

// Prune removes all stopped pods; the per-pod report (removed ids, per-item
// errors) is the mutation's success payload, rendered in place by the
// shared stay-open prune-report dialog.
export function usePrunePods(hostId: string) {
	return useListInvalidatingMutation(hostId, "pods", () => prunePods(hostId));
}
