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

export function usePods(machineId: string) {
	return useAlwaysRefetchListQuery(podsQueryKey(machineId), () =>
		listPods(machineId),
	);
}

export function usePod(machineId: string, podId: string) {
	return useOneShotQuery(podQueryKey(machineId, podId), () =>
		getPod(machineId, podId),
	);
}

export function usePodInspect(
	machineId: string,
	podId: string,
	options?: { enabled?: boolean },
) {
	return useOneShotQuery(
		podInspectQueryKey(machineId, podId),
		() => inspectPod(machineId, podId),
		options,
	);
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
