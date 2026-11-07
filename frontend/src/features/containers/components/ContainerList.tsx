import { Button, HStack, Td, Tr } from "@chakra-ui/react";
import {
	ContainerActionButtons,
	type ContainerLifecycleMutations,
} from "@features/containers/components/ContainerActionButtons";
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

import {
	ResourceList,
	type ResourceListSelectionSnapshot,
	snapshotSelection,
} from "@shared/components/ResourceList";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import { identityKey, rowScoped } from "@shared/queries/rowScopedMutation";
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
// and each row scopes its own pending state off the mutation's variables
// through the shared projection; both actions take the container id as
// their variables, so the key extractor is the shared identity. The record
// type is the shared action set's, so the row and the detail page hand over
// the same shape.

// The row is a plain function returning a Fragment of <Tr>s, not a
// component: the list's cell-cloning walk (mobile pinning and the selection
// checkbox column) reaches a row's cells only when `renderRow` hands back a
// `Tr` or a `Fragment` of `Tr`s — a component element is opaque to it, so a
// row wrapped in one silently loses both. The key rides the returned
// Fragment, which is the element the list's map places.
//
// Start/Stop and restart are scoped to the affected container by the
// mutation's own variables, so other rows stay usable. The buttons
// themselves are the shared action set's (spec #477), which the container
// detail page renders too. The row itself is clickable: it navigates to the
// container's detail page (spec 301), and every action button stops
// propagation so a button click never also fires the row action.
//
// The row carries no destructive button (spec 461, ticket #470): removal
// lives on the container's detail page (single item) and in the toolbar's
// "Remove selected" (bulk), so a mis-click on a row can never destroy a
// container. It carries no connect button either (spec #486, ticket #491):
// connecting is the toolbar's "Connect selected to network", which serves one
// selected row and many alike, so the action set is handed no `onConnect`.
function renderContainerRow({
	container,
	canManage,
	canManageNetworks,
	lifecycle,
	onOpen,
	onShowLogs,
}: {
	container: ContainerSummary;
	canManage: boolean;
	canManageNetworks: boolean;
	lifecycle: ContainerLifecycleMutations;
	onOpen: (container: ContainerSummary) => void;
	onShowLogs: (container: ContainerSummary) => void;
}) {
	const isRunning = container.state === "running";
	// A failed lifecycle action or restart shows the row-level error like
	// Stop does. Each button resets its sibling mutation on click, so a
	// stale error from the other action never masks the fresh one; the
	// variables keep another row's failure out of this row.
	const rowError =
		rowScoped(
			isRunning ? lifecycle.stop : lifecycle.start,
			container.id,
			identityKey,
		).error ?? rowScoped(lifecycle.restart, container.id, identityKey).error;

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
					<ContainerActionButtons
						container={container}
						name={containerDisplayName(container)}
						lifecycle={lifecycle}
						canManage={canManage}
						canManageNetworks={canManageNetworks}
						onShowLogs={() => onShowLogs(container)}
					/>
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
// (issue 256), and the bulk connect rides it (spec 486, ticket #491): the
// selection gate is the union of the two capabilities, so a network-only
// admin gets the checkboxes and the connect button without the
// container-write toolbar, and only a caller below both sees the hint. The toolbar/triad/table plumbing and the derived testids are
// ResourceList's; this adapter owns only the columns, the row, and the
// create/prune/bulk remove/bulk connect modals. The toolbar's connect button
// opens the networks feature's one connect dialog (issue 274) with the whole
// selection fixed — the containers feature calls into networks for connect,
// never the reverse.
//
// Removal has no per-row button (spec 461, ticket #470): a single container
// is removed from its detail page, several from the toolbar's "Remove
// selected". Connect follows the same shape (spec #486, ticket #491): the
// row's per-container button is gone, and the toolbar's "Connect selected to
// network" serves one selected row and many alike — one network chosen once
// for the selection, then a per-item report. Succeeded containers leave the
// selection; failed ones stay for the retry.
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
	// The selection gate is the union (spec 486, ticket #491): the bulk
	// connect rides manage-networks — the capability the row button used to
	// carry — while the toolbar's write actions (Create, Prune, Remove
	// selected) ride manage-containers. A network-only admin therefore
	// gets the checkboxes and the connect button without the container-write
	// toolbar; a caller below both capabilities keeps the permission hint.
	const canSelect = canManage || canManageNetworks;
	const navigate = useNavigate();
	// The bulk-connect dialog's snapshot: the selected containers plus the
	// selection's own partial clear, captured when the dialog opens. The list
	// owns the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and clears exactly the succeeded
	// keys when the run finishes, leaving the failures selected for retry.
	const [connectTarget, setConnectTarget] =
		useState<ResourceListSelectionSnapshot<ContainerSummary> | null>(null);
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
	// — get the same button feature-locally above the list. A network-only
	// admin's toolbar renders (the selection gate), so the fallback keys
	// on the same union. The shared module is deliberately not extended
	// for one ungated action.
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
			{canSelect ? null : (
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
					})
				}
				toolbar={
					<>
						{/* Create and Prune are container writes: a network-only
							admin's toolbar renders (the selection gate), so they
							gate themselves on the toolbar's write grant. */}
						{canManage ? (
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
							</>
						) : null}
						{statsButton}
					</>
				}
				canManage={canManage}
				// The selection gate: the union of the toolbar's write grant
				// and the bulk connect's own capability, so a network-only
				// admin can select and connect (spec 486, ticket #491).
				canSelect={canSelect}
				selection={{ getKey: (container) => container.id }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						containers: selection.selectedItems,
						clear: selection.clear,
					})
				}
				// The bulk connect sits beside Remove selected and is
				// permanently visible for a manage-networks caller — the
				// capability the row button used to carry — disabling itself
				// while nothing is selected. The module renders it for a
				// selection-capable caller only, so a read-level caller keeps
				// the permission-hint swap untouched.
				selectionActions={
					canManageNetworks
						? (selection) => (
								<Button
									size="sm"
									variant="outline"
									isDisabled={selection.selectedKeys.length === 0}
									onClick={() => setConnectTarget(snapshotSelection(selection))}
									data-testid="connect-selected-containers-button"
								>
									Connect selected to network
								</Button>
							)
						: undefined
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
			{connectTarget ? (
				<ConnectDialog
					hostId={hostId}
					containers={connectTarget.items}
					// The run's own seam: the succeeded containers leave the
					// selection, the failed ones stay for the retry.
					onFinished={connectTarget.clearKeys}
					onClose={() => setConnectTarget(null)}
				/>
			) : null}
		</>
	);
}
