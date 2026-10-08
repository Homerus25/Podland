import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { ConnectNetworkDialog } from "@features/containers/components/ConnectNetworkDialog";
import { CreateContainerDialog } from "@features/containers/components/CreateContainerDialog";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import {
	useContainers,
	useCreateContainer,
	useDeleteContainer,
	useStartContainer,
	useStopContainer,
} from "@features/containers/hooks/useContainers";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { can } from "@features/machines/permissions";
import { useConnectContainer } from "@features/networks/hooks/useNetworks";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";

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

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected container and other rows stay usable.
function ContainerRow({
	machineId,
	container,
	canManage,
	canManageNetworks,
	onDelete,
	onShowLogs,
	onConnect,
}: {
	machineId: string;
	container: ContainerSummary;
	canManage: boolean;
	canManageNetworks: boolean;
	onDelete: (container: ContainerSummary) => void;
	onShowLogs: (container: ContainerSummary) => void;
	onConnect: (container: ContainerSummary) => void;
}) {
	const startContainer = useStartContainer(machineId);
	const stopContainer = useStopContainer(machineId);
	const name = containerDisplayName(container);
	const action =
		container.state === "running"
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

	return (
		<>
			<Tr data-testid="container-row">
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
								onClick={() => action.lifecycle.mutate(container.id)}
								isLoading={action.lifecycle.isPending}
								data-testid={action.testId}
							>
								{action.label}
							</Button>
						) : null}
						{canManageNetworks ? (
							<Button
								size="xs"
								variant="outline"
								aria-label={`Connect to network ${name}`}
								onClick={() => onConnect(container)}
							>
								Connect to network
							</Button>
						) : null}
						<Button
							size="xs"
							variant="outline"
							aria-label={`Logs ${name}`}
							onClick={() => onShowLogs(container)}
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
							onClick={() => onDelete(container)}
						>
							Delete
						</Button>
					</Td>
				) : null}
			</Tr>
			{action.lifecycle.error ? (
				<Tr>
					<Td colSpan={canManage ? 5 : 4}>
						<ErrorDisplay error={getApiError(action.lifecycle.error)} />
					</Td>
				</Tr>
			) : null}
		</>
	);
}

// The Containers tab body. Create, start, stop, and delete are write-gated
// backend routes, so below manage level the module swaps the whole toolbar
// for the canonical read hint and the rows drop their start/stop/delete
// affordances; logs reads at read level, so the row keeps its logs button.
// Connect-to-network is gated on the separate manage-networks capability
// (issue 256): the row button hides below it while Logs stays visible. The
// toolbar/triad/table machinery and the derived testids are ResourceList's;
// this adapter owns only the columns, the row, and the create/delete/connect
// modals.
export function ContainerList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: containers, isPending, error } = useContainers(machineId);
	const deleteContainer = useDeleteContainer(machineId);
	const createContainer = useCreateContainer(machineId);
	const connectContainer = useConnectContainer(machineId);
	const canManage = can(machine, "manage-containers");
	const canManageNetworks = can(machine, "manage-networks");
	const [pendingDelete, setPendingDelete] = useState<ContainerSummary | null>(
		null,
	);
	const [pendingConnect, setPendingConnect] = useState<ContainerSummary | null>(
		null,
	);
	const openLogs = useOpenLogs(machineId, "container");
	const [isCreateOpen, setIsCreateOpen] = useState(false);

	return (
		<>
			<ResourceList
				resource="container"
				isPending={isPending}
				error={error}
				items={containers}
				columns={[
					{ label: "Name" },
					{ label: "Image" },
					{ label: "State" },
					{},
					...(canManage ? [{}] : []),
				]}
				renderRow={(container) => (
					<ContainerRow
						key={container.id}
						machineId={machineId}
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
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => setIsCreateOpen(true)}
						data-testid="create-container-button"
					>
						Create container
					</Button>
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
			{isCreateOpen ? (
				<CreateContainerDialog
					mutation={createContainer}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
			{pendingConnect ? (
				<ConnectNetworkDialog
					machineId={machineId}
					container={pendingConnect}
					mutation={connectContainer}
					onClose={() => setPendingConnect(null)}
				/>
			) : null}
		</>
	);
}
