import {
	Button,
	HStack,
	Spinner,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Th,
	Thead,
	Tr,
} from "@chakra-ui/react";
import {
	useContainerStats,
	useContainerStatsList,
} from "@features/containers/hooks/useContainers";
import type { ContainerStats } from "@features/containers/services/containersApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";

interface ContainerStatsDialogProps {
	machineId: string;
	onClose: () => void;
	// Present on the container details page: the dialog scopes to the one
	// container. Absent on the containers tab: the dialog shows every
	// running container on the machine.
	containerId?: string;
}

// One row of the snapshot table. Byte values render through the shared
// bytes formatter; CPU and memory percentages and the PID count render as
// the raw numbers they are on the wire.
function StatsRow({ stats }: { stats: ContainerStats }) {
	return (
		<Tr data-testid="container-stats-row">
			<Td fontFamily="mono">{stats.name}</Td>
			<Td isNumeric>{stats.cpuPercent}%</Td>
			<Td isNumeric>
				{formatBytes(stats.memoryUsage)} / {formatBytes(stats.memoryLimit)} (
				{stats.memoryPercent}%)
			</Td>
			<Td isNumeric>
				{formatBytes(stats.networkBytesIn)} in /{" "}
				{formatBytes(stats.networkBytesOut)} out
			</Td>
			<Td isNumeric>
				{formatBytes(stats.blockBytesIn)} in /{" "}
				{formatBytes(stats.blockBytesOut)} out
			</Td>
			<Td isNumeric>{stats.pids}</Td>
		</Tr>
	);
}

// The one-shot container stats dialog (spec 415), shared by the
// containers tab (all running containers) and the container details page
// (one container, via containerId). Stats is a read, so the dialog owns no
// mutation: it fetches its own snapshot query on open — the pages mount it
// conditionally, and the hooks' one-shot freshness keeps a reopen from
// serving the previous snapshot. The ~1s sample pause is the honest loading state; a
// stopped container's 404 (podman maps any first-report error to
// not-found) surfaces its message through the shared error envelope.
export function ContainerStatsDialog({
	machineId,
	containerId,
	onClose,
}: ContainerStatsDialogProps) {
	const scoped = containerId !== undefined;
	const listQuery = useContainerStatsList(machineId, { enabled: !scoped });
	const oneQuery = useContainerStats(machineId, containerId ?? "", {
		enabled: scoped,
	});
	const query = scoped ? oneQuery : listQuery;
	const rows = scoped
		? oneQuery.data
			? [oneQuery.data]
			: undefined
		: listQuery.data;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title="Container stats"
			footer={
				<Button onClick={onClose} data-testid="container-stats-close">
					Close
				</Button>
			}
		>
			{query.isPending ? (
				<HStack spacing={2} data-testid="container-stats-loading">
					<Spinner size="sm" />
					<Text>Loading container stats…</Text>
				</HStack>
			) : query.error ? (
				<ErrorDisplay
					error={getApiError(query.error)}
					data-testid="container-stats-error"
				/>
			) : rows && rows.length === 0 ? (
				<Text data-testid="container-stats-empty">
					No containers are running.
				</Text>
			) : rows ? (
				<TableContainer>
					<Table size="sm" data-testid="container-stats-table">
						<Thead>
							<Tr>
								<Th>Name</Th>
								<Th isNumeric>CPU</Th>
								<Th isNumeric>Memory</Th>
								<Th isNumeric>Network</Th>
								<Th isNumeric>Block</Th>
								<Th isNumeric>PIDs</Th>
							</Tr>
						</Thead>
						<Tbody>
							{rows.map((stats) => (
								<StatsRow key={stats.id} stats={stats} />
							))}
						</Tbody>
					</Table>
				</TableContainer>
			) : null}
		</Modal>
	);
}
