import { Button, HStack, Td, Tr } from "@chakra-ui/react";
import { ContainerStatsDialog } from "@features/containers/components/ContainerStatsDialog";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import {
	useBulkRemoveContainers,
	useContainers,
	usePruneContainers,
	useRestartContainer,
	useStartContainer,
	useStopContainer,
} from "@features/containers/hooks/useContainers";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { can } from "@features/hosts/permissions";
import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";

import { ResourceList } from "@shared/components/ResourceList";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import { containerCreatePath, detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { Fragment, useState } from "react";
import { useNavigate } from "react-router-dom";

// The prune adapter over PruneReportDialog, the twin of the volumes one:
// the module owns the plumbing, this adapter hands over the noun and the
// two entry accessors that make a container entry a report row. Podman's
// container prune report carries no name, so rows are keyed by ID like the
// pods one.
function PruneContainersModal({
	onClose,
	pruneContainers,
}: {
	onClose: () => void;
	pruneContainers: ReturnType<typeof usePruneContainers>;
}) {
	return (
		<PruneReportDialog
			mutation={pruneContainers}
			resource="container"
			entryKey={(entry) => entry.id}
			renderLabel={(entry) => entry.id}
			onClose={onClose}
		/>
	);
}

// The lifecycle mutations the list owns, one instance per action, handed to
// every row. A plain row function cannot own hooks, so the list keeps them
// and each row scopes its own pending state off the mutation's variables.
type ContainerLifecycleMutations = {
	start: ReturnType<typeof useStartContainer>;
	stop: ReturnType<typeof useStopContainer>;
	restart: ReturnType<typeof useRestartContainer>;
};

// The row is a plain function returning a Fragment of <Tr>s, not a
// component: the list's cell-cloning walk (mobile pinning and the selection
// checkbox column) reaches a row's cells only when `renderRow` hands back a
// `Tr` or a `Fragment` of `Tr`s — a component element is opaque to it, so a
// row wrapped in one silently loses both. The key rides the returned
// Fragment, which is the element the list's map places.
//
// Start/Stop and restart are scoped to the affected container by the
// mutation's own variables, so other rows stay usable. Restart is additive:
// it renders beside the Start/Stop toggle, only for running rows (issue
// #384). The row itself is clickable: it navigates to the container's detail
// page (spec 301), and every action button stops propagation so a button
// click never also fires the row action.
//
// The row carries no destructive button (spec 461, ticket #470): removal
// lives on the container's detail page (single item) and in the toolbar's
// "Remove selected" (bulk), so a mis-click on a row can never destroy a
// container.
function renderContainerRow({
	container,
	canManage,
	canManageNetworks,
	lifecycle,
	onOpen,
	onShowLogs,
	onConnect,
}: {
	container: ContainerSummary;
	canManage: boolean;
	canManageNetworks: boolean;
	lifecycle: ContainerLifecycleMutations;
	onOpen: (container: ContainerSummary) => void;
	onShowLogs: (container: ContainerSummary) => void;
	onConnect: (container: ContainerSummary) => void;
}) {
	const name = containerDisplayName(container);
	const isRunning = container.state === "running";
	const action = isRunning
		? {
				lifecycle: lifecycle.stop,
				testId: "stop-container-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: lifecycle.start,
				testId: "start-container-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	const actionPending =
		action.lifecycle.isPending && action.lifecycle.variables === container.id;
	const restartPending =
		lifecycle.restart.isPending && lifecycle.restart.variables === container.id;
	// A failed lifecycle action or restart shows the row-level error like
	// Stop does. Each button resets its sibling mutation on click, so a
	// stale error from the other action never masks the fresh one; the
	// variables keep another row's failure out of this row.
	const actionError =
		action.lifecycle.variables === container.id ? action.lifecycle.error : null;
	const restartError =
		lifecycle.restart.variables === container.id
			? lifecycle.restart.error
			: null;
	const rowError = actionError ?? restartError;

	return (
		<Fragment key={container.id}>
			<Tr
				data-testid="container-row"
				cursor="pointer"
				onClick={() => onOpen(container)}
			>
				<Td fontFamily="mono">{containerDisplayName(container)}</Td>
				<Td fontFamily="mono">{container.image}</Td>
				<Td>
					<StateBadge state={container.state} />
				</Td>
				<Td>
					<HStack spacing={2}>
						{canManage ? (
							<Button
								size="xs"
								variant="outline"
								colorScheme={action.colorScheme}
								onClick={(event) => {
									event.stopPropagation();
									lifecycle.restart.reset();
									action.lifecycle.mutate(container.id);
								}}
								isLoading={actionPending}
								data-testid={action.testId}
							>
								{action.label}
							</Button>
						) : null}
						{canManage && isRunning ? (
							<Button
								size="xs"
								variant="outline"
								aria-label={`Restart ${name}`}
								onClick={(event) => {
									event.stopPropagation();
									action.lifecycle.reset();
									lifecycle.restart.mutate(container.id);
								}}
								isLoading={restartPending}
								data-testid="restart-container-button"
							>
								Restart
							</Button>
						) : null}
						{canManageNetworks ? (
							<Button
								size="xs"
								variant="outline"
								aria-label={`Connect to network ${name}`}
								onClick={(event) => {
									event.stopPropagation();
									onConnect(container);
								}}
							>
								Connect to network
							</Button>
						) : null}
						<Button
							size="xs"
							variant="outline"
							aria-label={`Logs ${name}`}
							onClick={(event) => {
								event.stopPropagation();
								onShowLogs(container);
							}}
						>
							Logs
						</Button>
					</HStack>
				</Td>
			</Tr>
			{rowError ? (
				<Tr>
					<Td colSpan={4}>
						<ErrorDisplay error={getApiError(rowError)} />
					</Td>
				</Tr>
			) : null}
		</Fragment>
	);
}

// The Containers tab body. Create, start, stop, restart, prune, and bulk
// remove are write-gated backend routes, so below manage level the module
// swaps the whole toolbar for the canonical read hint and the rows drop
// their start/stop/restart affordances; logs reads at read level, so
// the row keeps its logs button.
// Connect-to-network is gated on the separate manage-networks capability
// (issue 256): the row button hides below it while Logs stays visible. The
// toolbar/triad/table plumbing and the derived testids are ResourceList's;
// this adapter owns only the columns, the row, and the create/prune/bulk
// remove modals. The row's connect button opens the networks feature's one
// connect dialog (issue 274) with the container fixed — the containers
// feature calls into networks for connect, never the reverse.
//
// Removal has no per-row button (spec 461, ticket #470): a single container
// is removed from its detail page, several from the toolbar's "Remove
// selected".
export function ContainerList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: containers, isPending, error } = useContainers(hostId);
	const pruneContainers = usePruneContainers(hostId);
	const bulkRemoveContainers = useBulkRemoveContainers(hostId);
	// One instance per lifecycle action, shared by every row: the row scopes
	// its own pending and error state off the mutation's variables.
	const lifecycle: ContainerLifecycleMutations = {
		start: useStartContainer(hostId),
		stop: useStopContainer(hostId),
		restart: useRestartContainer(hostId),
	};
	const canManage = can(host, "manage-containers");
	const canManageNetworks = can(host, "manage-networks");
	const navigate = useNavigate();
	const [pendingConnect, setPendingConnect] = useState<ContainerSummary | null>(
		null,
	);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [isStatsOpen, setIsStatsOpen] = useState(false);
	// The bulk-remove dialog's snapshot: the selected containers plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		containers: ContainerSummary[];
		clear: () => void;
	} | null>(null);
	const openLogs = useOpenLogs(hostId, "container");
	// Stats is a read, so the button must survive ResourceList's manage
	// gate: manage users get it in the toolbar beside Create/Prune, while
	// read users — whose toolbar the module swaps for the permission hint
	// — get the same button feature-locally above the list. The shared
	// module is deliberately not extended for one ungated action.
	const statsButton = (
		<Button
			size="sm"
			variant="outline"
			onClick={() => setIsStatsOpen(true)}
			data-testid="container-stats-button"
		>
			Stats
		</Button>
	);

	return (
		<>
			{canManage ? null : (
				<HStack justify="flex-end" mb={2}>
					{statsButton}
				</HStack>
			)}
			<ResourceList
				resource="container"
				isPending={isPending}
				error={error}
				items={containers}
				columns={[
					{
						label: "Name",
						isName: true,
						sortBy: (c) => containerDisplayName(c),
					},
					{ label: "Image", sortBy: (c) => c.image },
					{ label: "State", sortBy: (c) => c.state },
					{},
				]}
				renderRow={(container) =>
					renderContainerRow({
						container,
						canManage,
						canManageNetworks,
						lifecycle,
						onOpen: (target) =>
							navigate(detailPath(hostId, "containers", target.id)),
						onShowLogs: (target) =>
							openLogs({
								targetId: target.id,
								name: containerDisplayName(target),
							}),
						onConnect: setPendingConnect,
					})
				}
				toolbar={
					<>
						<Button
							size="sm"
							colorScheme="blue"
							onClick={() => navigate(containerCreatePath(hostId))}
							data-testid="create-container-button"
						>
							Create container
						</Button>
						<Button
							size="sm"
							colorScheme="red"
							variant="outline"
							onClick={() => setIsPruneOpen(true)}
							data-testid="prune-containers-button"
						>
							Prune
						</Button>
						{statsButton}
					</>
				}
				canManage={canManage}
				selection={{ getKey: (container) => container.id }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						containers: selection.selectedItems,
						clear: selection.clear,
					})
				}
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.containers}
					noun="container"
					itemKey={(container) => container.id}
					itemLabel={(container) => containerDisplayName(container)}
					mutation={bulkRemoveContainers}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
			{isPruneOpen ? (
				<PruneContainersModal
					onClose={() => setIsPruneOpen(false)}
					pruneContainers={pruneContainers}
				/>
			) : null}
			{isStatsOpen ? (
				<ContainerStatsDialog
					hostId={hostId}
					onClose={() => setIsStatsOpen(false)}
				/>
			) : null}
			{pendingConnect ? (
				<ConnectDialog
					hostId={hostId}
					container={pendingConnect}
					onClose={() => setPendingConnect(null)}
				/>
			) : null}
		</>
	);
}
