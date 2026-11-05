import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { ContainerStatsDialog } from "@features/containers/components/ContainerStatsDialog";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import {
	useContainers,
	useDeleteContainer,
	usePruneContainers,
	useRestartContainer,
	useStartContainer,
	useStopContainer,
} from "@features/containers/hooks/useContainers";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { can } from "@features/hosts/permissions";
import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import { containerCreatePath, detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// No delete request leaves the browser until the user explicitly confirms.
// The shell, reset lifecycle, danger footer, pending wiring, close-on-success,
// and error region are ConfirmDialog's; this adapter owns only the domain
// body and the variables (the container id). A failure (e.g. the 409
// CONTAINER_RUNNING envelope) keeps the modal open through the module's
// default error region.
function DeleteContainerModal({
	container,
	deleteContainer,
	onClose,
}: {
	container: ContainerSummary;
	deleteContainer: ReturnType<typeof useDeleteContainer>;
	onClose: () => void;
}) {
	return (
		<ConfirmDialog
			mutation={deleteContainer}
			title="Delete container"
			confirmLabel="Delete container"
			confirm={() => container.id}
			onClose={onClose}
			body={
				<Text>
					Delete “{containerDisplayName(container)}” ({container.image})? This
					cannot be undone.
				</Text>
			}
		/>
	);
}

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

// Each row owns its lifecycle mutation instances (start/stop and
// restart), so pending state and errors are scoped to the affected
// container and other rows stay usable. Restart is additive: it renders
// beside the Start/Stop toggle, only for running rows (issue #384).
// The row itself is clickable: it navigates to the container's detail page
// (spec 301), and every action button stops propagation so a button click
// never also fires the row action.
function ContainerRow({
	hostId,
	container,
	canManage,
	canManageNetworks,
	onDelete,
	onShowLogs,
	onConnect,
}: {
	hostId: string;
	container: ContainerSummary;
	canManage: boolean;
	canManageNetworks: boolean;
	onDelete: (container: ContainerSummary) => void;
	onShowLogs: (container: ContainerSummary) => void;
	onConnect: (container: ContainerSummary) => void;
}) {
	const navigate = useNavigate();
	const startContainer = useStartContainer(hostId);
	const stopContainer = useStopContainer(hostId);
	const restartContainer = useRestartContainer(hostId);
	const name = containerDisplayName(container);
	const isRunning = container.state === "running";
	const action = isRunning
		? {
				lifecycle: stopContainer,
				testId: "stop-container-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: startContainer,
				testId: "start-container-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	// A failed lifecycle action or restart shows the row-level error like
	// Stop does. Each button resets its sibling mutation on click, so a
	// stale error from the other action never masks the fresh one.
	const rowError = action.lifecycle.error ?? restartContainer.error;

	return (
		<>
			<Tr
				data-testid="container-row"
				cursor="pointer"
				onClick={() => navigate(detailPath(hostId, "containers", container.id))}
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
									restartContainer.reset();
									action.lifecycle.mutate(container.id);
								}}
								isLoading={action.lifecycle.isPending}
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
									restartContainer.mutate(container.id);
								}}
								isLoading={restartContainer.isPending}
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
				{canManage ? (
					<Td>
						<Button
							size="xs"
							colorScheme="red"
							variant="outline"
							aria-label={`Delete ${name}`}
							onClick={(event) => {
								event.stopPropagation();
								onDelete(container);
							}}
						>
							Delete
						</Button>
					</Td>
				) : null}
			</Tr>
			{rowError ? (
				<Tr>
					<Td colSpan={canManage ? 5 : 4}>
						<ErrorDisplay error={getApiError(rowError)} />
					</Td>
				</Tr>
			) : null}
		</>
	);
}

// The Containers tab body. Create, start, stop, restart, delete, and
// prune are
// write-gated backend routes, so below manage level the module swaps the
// whole toolbar for the canonical read hint and the rows drop their
// start/stop/restart/delete affordances; logs reads at read level, so
// the row keeps its logs button.
// Connect-to-network is gated on the separate manage-networks capability
// (issue 256): the row button hides below it while Logs stays visible. The
// toolbar/triad/table plumbing and the derived testids are ResourceList's;
// this adapter owns only the columns, the row, and the create/delete
// modals. The row's connect button opens the networks feature's one
// connect dialog (issue 274) with the container fixed — the containers
// feature calls into networks for connect, never the reverse.
export function ContainerList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: containers, isPending, error } = useContainers(hostId);
	const deleteContainer = useDeleteContainer(hostId);
	const pruneContainers = usePruneContainers(hostId);
	const canManage = can(host, "manage-containers");
	const canManageNetworks = can(host, "manage-networks");
	const navigate = useNavigate();
	const [pendingDelete, setPendingDelete] = useState<ContainerSummary | null>(
		null,
	);
	const [pendingConnect, setPendingConnect] = useState<ContainerSummary | null>(
		null,
	);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [isStatsOpen, setIsStatsOpen] = useState(false);
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
					...(canManage ? [{}] : []),
				]}
				renderRow={(container) => (
					<ContainerRow
						key={container.id}
						hostId={hostId}
						container={container}
						canManage={canManage}
						canManageNetworks={canManageNetworks}
						onDelete={setPendingDelete}
						onShowLogs={(container) =>
							openLogs({
								targetId: container.id,
								name: containerDisplayName(container),
							})
						}
						onConnect={setPendingConnect}
					/>
				)}
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
			/>
			{pendingDelete ? (
				<DeleteContainerModal
					container={pendingDelete}
					deleteContainer={deleteContainer}
					onClose={() => setPendingDelete(null)}
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
