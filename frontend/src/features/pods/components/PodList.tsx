import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CreatePodDialog } from "@features/pods/components/CreatePodDialog";
import {
	useBulkRemovePods,
	useCreatePod,
	usePodAction,
	usePods,
	usePrunePods,
} from "@features/pods/hooks/usePods";
import type {
	PodPruneEntry,
	PodSummary,
} from "@features/pods/services/podsApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";

import { ResourceList } from "@shared/components/ResourceList";
import { useOpenLogs } from "@shared/logs/logRoute";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { formatRelativeDate } from "@shared/utils/format";
import { Fragment, useState } from "react";
import { useNavigate } from "react-router-dom";

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

// The list owns one mutation instance per lifecycle action — a plain row
// function cannot own hooks — so a row's pending and error state are scoped
// by the mutation's own variables: only the row whose id the in-flight (or
// last-failed) call carried shows them, and the other rows stay usable.
type PodActionMutations = {
	start: ReturnType<typeof usePodAction>;
	stop: ReturnType<typeof usePodAction>;
	pause: ReturnType<typeof usePodAction>;
	unpause: ReturnType<typeof usePodAction>;
	kill: ReturnType<typeof usePodAction>;
	restart: ReturnType<typeof usePodAction>;
};

function rowScoped(mutation: PodActionMutations["start"], podId: string) {
	const mine = mutation.variables === podId;
	return {
		isPending: mutation.isPending && mine,
		error: mine ? mutation.error : null,
	};
}

// The row is a plain function returning a Fragment of <Tr>s, not a
// component: the list's cell-cloning walk (mobile pinning and the selection
// checkbox column) reaches a row's cells only when `renderRow` hands back a
// `Tr` or a `Fragment` of `Tr`s — a component element is opaque to it, so a
// row wrapped in one silently loses both. The key rides the returned
// Fragment, which is the element the list's map places.
//
// The row itself is clickable: it navigates to the pod's detail page, and
// every action button stops propagation so a button click never also fires
// the row action. Start/Stop is one toggle whose label follows the pod's
// state, mirroring the container lifecycle buttons; Pause/Unpause is one
// button whose label follows the pod's state; kill and restart take no
// parameters (podman's defaults). The lifecycle affordances are write-gated,
// but logs reads at read level, so the row keeps its logs button below the
// manage capability. The row carries no destructive button (spec 461, ticket
// #470): removal lives on the pod's detail page and in the toolbar's "Remove
// selected".
function renderPodRow({
	pod,
	canManage,
	actions,
	onOpen,
	onShowLogs,
}: {
	pod: PodSummary;
	canManage: boolean;
	actions: PodActionMutations;
	onOpen: (pod: PodSummary) => void;
	onShowLogs: (pod: PodSummary) => void;
}) {
	// Podman's pod status is capitalized ("Paused"); compare case-insensitively.
	const paused = pod.state.toLowerCase() === "paused";
	const pauseAction = paused ? actions.unpause : actions.pause;
	// Podman's pod status is capitalized ("Running"); compare
	// case-insensitively, like the pause toggle above.
	const isRunning = pod.state.toLowerCase() === "running";
	const action = isRunning
		? {
				lifecycle: actions.stop,
				testId: "stop-pod-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: actions.start,
				testId: "start-pod-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	// The toggle's sibling mutation resets on each click, so a stale error
	// from the other action never masks the fresh one; a failed start or
	// stop shows the row-level error like the containers surface does.
	const sibling = isRunning ? actions.start : actions.stop;
	const lifecycle = rowScoped(action.lifecycle, pod.id);
	const pauseState = rowScoped(pauseAction, pod.id);
	const killState = rowScoped(actions.kill, pod.id);
	const restartState = rowScoped(actions.restart, pod.id);
	const rowError = lifecycle.error;

	return (
		<Fragment key={pod.id}>
			<Tr data-testid="pod-row" cursor="pointer" onClick={() => onOpen(pod)}>
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
									isLoading={lifecycle.isPending}
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
									isLoading={pauseState.isPending}
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
									isLoading={killState.isPending}
									onClick={(event) => {
										event.stopPropagation();
										actions.kill.mutate(pod.id);
									}}
									data-testid="kill-pod-button"
								>
									Kill
								</Button>
								<Button
									size="xs"
									variant="outline"
									aria-label={`Restart ${pod.name}`}
									isLoading={restartState.isPending}
									onClick={(event) => {
										event.stopPropagation();
										actions.restart.mutate(pod.id);
									}}
									data-testid="restart-pod-button"
								>
									Restart
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
		</Fragment>
	);
}

// The Pods tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on the host's per-caller
// facts. The toolbar/triad/table plumbing and the hint swap are
// ResourceList's; this adapter owns only the columns, the row, and the
// create/prune/bulk-remove modals (below the manage capability, the hint
// replaces the controls — never bait users with disabled buttons).
//
// Removal has no per-row button (spec 461, ticket #470): a single pod is
// removed from its detail page, several from the toolbar's "Remove
// selected".
export function PodList({ host }: { host: Host }) {
	const hostId = host.id;
	const navigate = useNavigate();
	const { data: pods, isPending, error } = usePods(hostId);
	const createPod = useCreatePod(hostId);
	const prunePods = usePrunePods(hostId);
	const bulkRemovePods = useBulkRemovePods(hostId);
	// One instance per lifecycle action, shared by every row: the row scopes
	// its own pending and error state off the mutation's variables.
	const actions: PodActionMutations = {
		start: usePodAction(hostId, "start"),
		stop: usePodAction(hostId, "stop"),
		pause: usePodAction(hostId, "pause"),
		unpause: usePodAction(hostId, "unpause"),
		kill: usePodAction(hostId, "kill"),
		restart: usePodAction(hostId, "restart"),
	};
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	// The bulk-remove dialog's snapshot: the selected pods plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		pods: PodSummary[];
		clear: () => void;
	} | null>(null);
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
				renderRow={(pod) =>
					renderPodRow({
						pod,
						canManage,
						actions,
						onOpen: (target) => navigate(detailPath(hostId, "pods", target.id)),
						onShowLogs: (target) =>
							openLogs({ targetId: target.id, name: target.name }),
					})
				}
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
				selection={{ getKey: (pod) => pod.id }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						pods: selection.selectedItems,
						clear: selection.clear,
					})
				}
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.pods}
					noun="pod"
					itemKey={(pod) => pod.id}
					itemLabel={(pod) => pod.name}
					mutation={bulkRemovePods}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
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
		</>
	);
}
