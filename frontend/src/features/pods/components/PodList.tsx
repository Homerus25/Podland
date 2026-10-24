import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import { CreatePodDialog } from "@features/pods/components/CreatePodDialog";
import {
	useCreatePod,
	usePodAction,
	usePods,
	usePrunePods,
	useRemovePod,
} from "@features/pods/hooks/usePods";
import type {
	PodPruneEntry,
	PodSummary,
} from "@features/pods/services/podsApi";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Machine } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The remove adapter over the shared force-refusal preset: the module owns
// the confirm-to-force ritual (the running hint, the footer swap, the force
// flag keyed off the current error); this adapter keeps its domain only —
// the POD_STATE_CONFLICT refusal code, the hint copy, the labels, the
// testids, and the payload shape.
function RemovePodModal({
	pod,
	onClose,
	removePod,
}: {
	pod: PodSummary;
	onClose: () => void;
	removePod: ReturnType<typeof useRemovePod>;
}) {
	return (
		<ForceConfirmDialog
			mutation={removePod}
			title="Remove pod"
			onClose={onClose}
			refusalCode="POD_STATE_CONFLICT"
			hint="This pod has running containers, so it cannot be removed normally. Force remove also removes its running containers; they may lose work."
			hintTestId="pod-running-warning"
			confirmLabel="Remove pod"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ podId: pod.id, force })}
			body={<Text>Remove pod “{pod.name}”? This cannot be undone.</Text>}
		/>
	);
}

// The prune adapter over PruneReportDialog. The module owns the reset
// lifecycle, the confirm/report footer swap, the report table, and the
// noun-derived copy; this adapter hands over the mutation result plus the
// entry accessors that make a pod entry a report row. Pods report no
// reclaimed size, so the report's reclaimed column renders an em dash.
function PrunePodsModal({
	onClose,
	prunePods,
}: {
	onClose: () => void;
	prunePods: ReturnType<typeof usePrunePods>;
}) {
	return (
		<PruneReportDialog<PodPruneEntry>
			mutation={prunePods}
			resource="pod"
			entryKey={(entry) => entry.id}
			renderLabel={(entry) => entry.id}
			onClose={onClose}
		/>
	);
}

// Each row owns its lifecycle mutation instances, so pending state and
// errors stay scoped to the affected pod and other rows stay usable. The
// row itself is clickable: it navigates to the pod's detail page, and every
// action button stops propagation so a button click never also fires the
// row action. Pause/Unpause is one button whose label follows the pod's
// state; kill and restart take no parameters (podman's defaults).
function PodRow({
	machineId,
	pod,
	canManage,
	onRemove,
}: {
	machineId: string;
	pod: PodSummary;
	canManage: boolean;
	onRemove: (pod: PodSummary) => void;
}) {
	const navigate = useNavigate();
	const pause = usePodAction(machineId, "pause");
	const unpause = usePodAction(machineId, "unpause");
	const kill = usePodAction(machineId, "kill");
	const restart = usePodAction(machineId, "restart");
	// Podman's pod status is capitalized ("Paused"); compare case-insensitively.
	const paused = pod.state.toLowerCase() === "paused";
	const pauseAction = paused ? unpause : pause;

	return (
		<Tr
			data-testid="pod-row"
			cursor="pointer"
			onClick={() => navigate(detailPath(machineId, "pods", pod.id))}
		>
			<Td fontFamily="mono">{pod.name}</Td>
			<Td>
				<Text data-testid={`pod-state-${pod.id}`}>{pod.state}</Text>
			</Td>
			<Td isNumeric>{pod.containerCount}</Td>
			<Td fontFamily="mono" color="gray.500">
				{(pod.networks ?? []).join(", ")}
			</Td>
			<Td>{formatRelativeDate(pod.createdAt)}</Td>
			{canManage ? (
				<Td>
					<HStack spacing={2}>
						<Button
							size="xs"
							variant="outline"
							aria-label={`${paused ? "Unpause" : "Pause"} ${pod.name}`}
							isLoading={pauseAction.isPending}
							onClick={(event) => {
								event.stopPropagation();
								pauseAction.mutate(pod.id);
							}}
							data-testid={paused ? "unpause-pod-button" : "pause-pod-button"}
						>
							{paused ? "Unpause" : "Pause"}
						</Button>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Kill ${pod.name}`}
							isLoading={kill.isPending}
							onClick={(event) => {
								event.stopPropagation();
								kill.mutate(pod.id);
							}}
							data-testid="kill-pod-button"
						>
							Kill
						</Button>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Restart ${pod.name}`}
							isLoading={restart.isPending}
							onClick={(event) => {
								event.stopPropagation();
								restart.mutate(pod.id);
							}}
							data-testid="restart-pod-button"
						>
							Restart
						</Button>
						<Button
							size="xs"
							colorScheme="red"
							variant="outline"
							aria-label={`Remove ${pod.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onRemove(pod);
							}}
							data-testid="remove-pod-button"
						>
							Remove
						</Button>
					</HStack>
				</Td>
			) : null}
		</Tr>
	);
}

// The Pods tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on the machine's per-caller
// facts. The toolbar/triad/table machinery and the hint swap are
// ResourceList's; this adapter owns only the columns, the row, and the
// create/prune/remove modals (below the manage capability, the hint
// replaces the controls — never bait users with disabled buttons).
export function PodList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: pods, isPending, error } = usePods(machineId);
	const createPod = useCreatePod(machineId);
	const removePod = useRemovePod(machineId);
	const prunePods = usePrunePods(machineId);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [pendingRemove, setPendingRemove] = useState<PodSummary | null>(null);
	const canManage = can(machine, "manage-pods");

	return (
		<>
			<ResourceList
				resource="pod"
				isPending={isPending}
				error={error}
				items={pods}
				columns={[
					{ label: "Name", sortBy: (pod) => pod.name },
					{ label: "State", sortBy: (pod) => pod.state },
					{
						label: "Containers",
						isNumeric: true,
						sortBy: (pod) => pod.containerCount,
					},
					{
						label: "Networks",
						sortBy: (pod) => (pod.networks ?? []).join(", "),
					},
					{ label: "Created", sortBy: (pod) => pod.createdAt },
					...(canManage ? [{}] : []),
				]}
				renderRow={(pod) => (
					<PodRow
						key={pod.id}
						machineId={machineId}
						pod={pod}
						canManage={canManage}
						onRemove={setPendingRemove}
					/>
				)}
				toolbar={
					<>
						<Button
							size="sm"
							colorScheme="blue"
							onClick={() => setIsCreateOpen(true)}
							data-testid="create-pod-button"
						>
							Create pod
						</Button>
						<Button
							size="sm"
							colorScheme="red"
							variant="outline"
							onClick={() => setIsPruneOpen(true)}
							data-testid="prune-pods-button"
						>
							Prune unused
						</Button>
					</>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<CreatePodDialog
					mutation={createPod}
					machineId={machineId}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
			{isPruneOpen ? (
				<PrunePodsModal
					onClose={() => setIsPruneOpen(false)}
					prunePods={prunePods}
				/>
			) : null}
			{pendingRemove ? (
				<RemovePodModal
					pod={pendingRemove}
					onClose={() => setPendingRemove(null)}
					removePod={removePod}
				/>
			) : null}
		</>
	);
}
