import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
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
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { useOpenLogs } from "@shared/logs/logRoute";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
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
// row action. Start/Stop is one toggle whose label follows the pod's
// state, mirroring the container lifecycle buttons; Pause/Unpause is one
// button whose label follows the pod's state; kill and restart take no
// parameters (podman's defaults). The lifecycle affordances are
// write-gated, but logs reads at read level, so the row keeps its logs
// button below the manage capability.
function PodRow({
	hostId,
	pod,
	canManage,
	onRemove,
	onShowLogs,
}: {
	hostId: string;
	pod: PodSummary;
	canManage: boolean;
	onRemove: (pod: PodSummary) => void;
	onShowLogs: (pod: PodSummary) => void;
}) {
	const navigate = useNavigate();
	const start = usePodAction(hostId, "start");
	const stop = usePodAction(hostId, "stop");
	const pause = usePodAction(hostId, "pause");
	const unpause = usePodAction(hostId, "unpause");
	const kill = usePodAction(hostId, "kill");
	const restart = usePodAction(hostId, "restart");
	// Podman's pod status is capitalized ("Paused"); compare case-insensitively.
	const paused = pod.state.toLowerCase() === "paused";
	const pauseAction = paused ? unpause : pause;
	// Podman's pod status is capitalized ("Running"); compare
	// case-insensitively, like the pause toggle above.
	const isRunning = pod.state.toLowerCase() === "running";
	const action = isRunning
		? {
				lifecycle: stop,
				testId: "stop-pod-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: start,
				testId: "start-pod-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	// The toggle's sibling mutation resets on each click, so a stale error
	// from the other action never masks the fresh one; a failed start or
	// stop shows the row-level error like the containers surface does.
	const sibling = isRunning ? start : stop;
	const rowError = action.lifecycle.error;

	return (
		<>
			<Tr
				data-testid="pod-row"
				cursor="pointer"
				onClick={() => navigate(detailPath(hostId, "pods", pod.id))}
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
				<Td>
					<HStack spacing={2}>
						{canManage ? (
							<>
								<Button
									size="xs"
									variant="outline"
									colorScheme={action.colorScheme}
									aria-label={`${action.label} ${pod.name}`}
									isLoading={action.lifecycle.isPending}
									onClick={(event) => {
										event.stopPropagation();
										sibling.reset();
										action.lifecycle.mutate(pod.id);
									}}
									data-testid={action.testId}
								>
									{action.label}
								</Button>
								<Button
									size="xs"
									variant="outline"
									aria-label={`${paused ? "Unpause" : "Pause"} ${pod.name}`}
									isLoading={pauseAction.isPending}
									onClick={(event) => {
										event.stopPropagation();
										pauseAction.mutate(pod.id);
									}}
									data-testid={
										paused ? "unpause-pod-button" : "pause-pod-button"
									}
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
							</>
						) : null}
						<Button
							size="xs"
							variant="outline"
							aria-label={`Logs ${pod.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onShowLogs(pod);
							}}
						>
							Logs
						</Button>
					</HStack>
				</Td>
			</Tr>
			{rowError ? (
				<Tr>
					<Td colSpan={6}>
						<ErrorDisplay error={getApiError(rowError)} />
					</Td>
				</Tr>
			) : null}
		</>
	);
}

// The Pods tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on the host's per-caller
// facts. The toolbar/triad/table plumbing and the hint swap are
// ResourceList's; this adapter owns only the columns, the row, and the
// create/prune/remove modals (below the manage capability, the hint
// replaces the controls — never bait users with disabled buttons).
export function PodList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: pods, isPending, error } = usePods(hostId);
	const createPod = useCreatePod(hostId);
	const removePod = useRemovePod(hostId);
	const prunePods = usePrunePods(hostId);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [pendingRemove, setPendingRemove] = useState<PodSummary | null>(null);
	const canManage = can(host, "manage-pods");
	const openLogs = useOpenLogs(hostId, "pod");

	return (
		<>
			<ResourceList
				resource="pod"
				isPending={isPending}
				error={error}
				items={pods}
				columns={[
					{ label: "Name", isName: true, sortBy: (pod) => pod.name },
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
					{},
				]}
				renderRow={(pod) => (
					<PodRow
						key={pod.id}
						hostId={hostId}
						pod={pod}
						canManage={canManage}
						onRemove={setPendingRemove}
						onShowLogs={(pod) => openLogs({ targetId: pod.id, name: pod.name })}
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
							Prune
						</Button>
					</>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<CreatePodDialog
					mutation={createPod}
					hostId={hostId}
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
