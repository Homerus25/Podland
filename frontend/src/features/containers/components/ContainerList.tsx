import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
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
import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import { detailPath } from "@shared/routes/detailRoute";
import type { Machine } from "@shared/types/api";
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

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected container and other rows stay usable.
// The row itself is clickable: it navigates to the container's detail page
// (spec 301), and every action button stops propagation so a button click
// never also fires the row action.
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
	const navigate = useNavigate();
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
			<Tr
				data-testid="container-row"
				cursor="pointer"
				onClick={() =>
					navigate(detailPath(machineId, "containers", container.id))
				}
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
									action.lifecycle.mutate(container.id);
								}}
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
// this adapter owns only the columns, the row, and the create/delete
// modals. The row's connect button opens the networks feature's one
// connect dialog (issue 274) with the container fixed — the containers
// feature calls into networks for connect, never the reverse.
export function ContainerList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: containers, isPending, error } = useContainers(machineId);
	const deleteContainer = useDeleteContainer(machineId);
	const createContainer = useCreateContainer(machineId);
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
					{ label: "Name", sortBy: (c) => containerDisplayName(c) },
					{ label: "Image", sortBy: (c) => c.image },
					{ label: "State", sortBy: (c) => c.state },
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
				<ConnectDialog
					machineId={machineId}
					container={pendingConnect}
					onClose={() => setPendingConnect(null)}
				/>
			) : null}
		</>
	);
}
