import {
	createVolume,
	deleteVolume,
	getVolumeMovePreview,
	listVolumes,
	populateVolume,
	pruneVolumes,
	type VolumeCreateInput,
	type VolumeMoveResult,
} from "@features/volumes/services/volumesApi";
import {
	volumeMovePreviewQueryKey,
	volumesQueryKey,
} from "@shared/queries/queryKeys";
import {
	invalidateMachineResources,
	useListInvalidatingMutation,
} from "@shared/queries/useListInvalidatingMutation";
import { type QueryClient, useQuery } from "@tanstack/react-query";

export function useVolumes(machineId: string) {
	return useQuery({
		queryKey: volumesQueryKey(machineId),
		queryFn: () => listVolumes(machineId),
	});
}

// A create extends the machine's volume list, so the list refetches on
// success and shows the server's post-action state; no optimistic updates.
export function useCreateVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		(input: VolumeCreateInput) => createVolume(machineId, input),
	);
}

// A delete removes one volume from the machine's list, so the list
// refetches on success; no optimistic updates.
export function useDeleteVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		({ volumeName, force }: { volumeName: string; force: boolean }) =>
			deleteVolume(machineId, volumeName, force),
	);
}

// A prune changes the machine's volume set, so the list refetches on
// success and the report shows the server's post-prune state.
export function usePruneVolumes(machineId: string) {
	return useListInvalidatingMutation(machineId, "volumes", () =>
		pruneVolumes(machineId),
	);
}

// Populate changes a volume's contents (and reported size), so the list
// refetches on success; no optimistic updates.
export function usePopulateVolume(machineId: string) {
	return useListInvalidatingMutation(
		machineId,
		"volumes",
		(input: {
			volumeName: string;
			form: FormData;
			onProgress?: (fraction: number) => void;
		}) =>
			populateVolume(machineId, input.volumeName, input.form, input.onProgress),
	);
}

// The transfer dialog's fact payload, keyed by the chosen target so a
// target change refetches. Disabled until a target exists — the dialog
// defaults the select to the first writable machine once the machines list
// arrives.
export function useVolumeMovePreview(
	machineId: string,
	volumeName: string,
	targetMachineId: string | null,
) {
	return useQuery({
		queryKey: volumeMovePreviewQueryKey(
			machineId,
			volumeName,
			targetMachineId ?? "",
		),
		queryFn: () => {
			if (targetMachineId === null) {
				throw new Error("volume move preview requires a target machine");
			}
			return getVolumeMovePreview(machineId, volumeName, targetMachineId);
		},
		enabled: targetMachineId !== null,
	});
}

// A volume transfer touches two machines: the target gains the volume's
// data and (in move mode) the source loses the volume. The envelope
// carries both machine ids, so both lists invalidate from what the server
// answered rather than from closure assumptions. A "partial" result is
// still a success-shaped answer — the data crossed — so both lists refresh
// there too. One owner for the rule: the stream transport's complete
// handler calls this (the quadlet move's invalidateMoveResult pattern).
export function invalidateVolumeMoveResult(
	queryClient: QueryClient,
	result: VolumeMoveResult,
) {
	invalidateMachineResources(queryClient, result.sourceMachineId, "volumes");
	invalidateMachineResources(queryClient, result.targetMachineId, "volumes");
}
