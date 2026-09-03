import {
	Badge,
	Box,
	Button,
	Checkbox,
	HStack,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Th,
	Thead,
	Tr,
	useToast,
} from "@chakra-ui/react";
import {
	canManageVolumes,
	type EffectivePermission,
} from "@features/machines/permissions";
import { VolumeForm } from "@features/volumes/components/VolumeForm";
import {
	useCreateVolume,
	useDeleteVolume,
	usePruneVolumes,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import type {
	VolumePruneReport,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { mutationToPromise } from "@shared/utils/mutationPromise";
import { useState } from "react";
import { VolumeInspectDrawer } from "./VolumeInspectDrawer";

function VolumeRow({
	volume,
	onSelect,
	canManage,
	onDelete,
	isExporting,
	onExport,
}: {
	volume: VolumeSummary;
	onSelect: (name: string) => void;
	canManage: boolean;
	onDelete: (volume: VolumeSummary) => void;
	isExporting: boolean;
	onExport: (volume: VolumeSummary) => void;
}) {
	return (
		<Tr
			data-testid="volume-row"
			cursor="pointer"
			onClick={() => onSelect(volume.name)}
		>
			<Td fontFamily="mono">{volume.name}</Td>
			<Td>
				<Badge
					colorScheme={volume.anonymous ? "gray" : "purple"}
					data-testid="volume-type-badge"
				>
					{volume.anonymous ? "Anonymous" : "Named"}
				</Badge>
			</Td>
			<Td>{volume.driver}</Td>
			<Td>{formatRelativeDate(volume.createdAt)}</Td>
			<Td isNumeric>
				{volume.size !== undefined ? formatBytes(volume.size) : "—"}
			</Td>
			<Td>
				<Button
					size="xs"
					variant="outline"
					aria-label={`Export ${volume.name}`}
					isLoading={isExporting}
					onClick={(event) => {
						event.stopPropagation();
						onExport(volume);
					}}
				>
					Export
				</Button>
				{canManage ? (
					<Button
						size="xs"
						colorScheme="red"
						variant="outline"
						aria-label={`Delete ${volume.name}`}
						onClick={(event) => {
							event.stopPropagation();
							onDelete(volume);
						}}
					>
						Delete
					</Button>
				) : null}
			</Td>
		</Tr>
	);
}

const VOLUME_IN_USE_CODE = "VOLUME_IN_USE";

function isInUse(error: unknown): boolean {
	return getApiError(error).code === VOLUME_IN_USE_CODE;
}

function DeleteVolumeModal({
	volume,
	onClose,
	deleteVolume,
}: {
	volume: VolumeSummary;
	onClose: () => void;
	deleteVolume: ReturnType<typeof useDeleteVolume>;
}) {
	const [force, setForce] = useState(false);
	const toast = useToast();
	const inUse = isInUse(deleteVolume.error);

	return (
		<Modal
			isOpen
			onClose={onClose}
			title="Delete volume"
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					<Button
						colorScheme="red"
						onClick={() =>
							deleteVolume.mutate(
								{ volumeName: volume.name, force },
								{
									onSuccess: () => {
										toast({
											title: `Volume deleted: ${volume.name}`,
											status: "success",
											isClosable: true,
										});
										onClose();
									},
									onError: () => {
										toast({
											title: `Failed to delete volume: ${volume.name}`,
											status: "error",
											isClosable: true,
										});
									},
								},
							)
						}
						isLoading={deleteVolume.isPending}
						data-testid="confirm-delete-button"
					>
						Delete volume
					</Button>
				</>
			}
		>
			<Text>
				Delete volume {volume.name} from this machine? This cannot be undone.
			</Text>
			<Checkbox
				mt={3}
				isChecked={force}
				onChange={(event) => setForce(event.target.checked)}
				data-testid="force-delete-checkbox"
			>
				Force delete (stops containers using this volume before removal)
			</Checkbox>
			{inUse ? (
				<>
					<Box mt={2}>
						<ErrorDisplay error={getApiError(deleteVolume.error)} />
					</Box>
					<Text mt={2} color="orange.500" data-testid="volume-in-use-warning">
						Force is required to delete a volume that containers are using. Tick
						force delete and confirm to stop them and remove the volume.
					</Text>
				</>
			) : deleteVolume.error ? (
				<Box mt={2}>
					<ErrorDisplay error={getApiError(deleteVolume.error)} />
				</Box>
			) : null}
		</Modal>
	);
}

function PruneReportList({ report }: { report: VolumePruneReport[] }) {
	if (report.length === 0) {
		return (
			<Text data-testid="prune-report-empty">
				Nothing to prune: no unused volumes on this machine.
			</Text>
		);
	}
	return (
		<TableContainer>
			<Table data-testid="prune-report" size="sm">
				<Thead>
					<Tr>
						<Th>Removed volume</Th>
						<Th isNumeric>Reclaimed</Th>
					</Tr>
				</Thead>
				<Tbody>
					{report.map((entry) => (
						<Tr key={entry.name} data-testid="prune-report-row">
							<Td fontFamily="mono">{entry.name}</Td>
							{entry.error ? (
								<Td color="red.500">{entry.error}</Td>
							) : (
								<Td isNumeric>{formatBytes(entry.size)}</Td>
							)}
						</Tr>
					))}
				</Tbody>
			</Table>
		</TableContainer>
	);
}

function PruneVolumesModal({
	onClose,
	pruneVolumes,
}: {
	onClose: () => void;
	pruneVolumes: ReturnType<typeof usePruneVolumes>;
}) {
	const report = pruneVolumes.data;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={report ? "Prune report" : "Prune unused volumes"}
			footer={
				report ? (
					<Button onClick={onClose} data-testid="close-prune-report-button">
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme="red"
							onClick={() => pruneVolumes.mutate()}
							isLoading={pruneVolumes.isPending}
							data-testid="confirm-prune-button"
						>
							Prune volumes
						</Button>
					</>
				)
			}
		>
			{report ? (
				<PruneReportList report={report} />
			) : (
				<>
					<Text>
						All unused volumes on this machine will be removed. This cannot be
						undone.
					</Text>
					<Text mt={2} color="gray.500">
						Volumes that are in use by containers are safe and will not be
						touched.
					</Text>
					{pruneVolumes.error ? (
						<Box mt={2}>
							<ErrorDisplay error={getApiError(pruneVolumes.error)} />
						</Box>
					) : null}
				</>
			)}
		</Modal>
	);
}

export function VolumeList({
	machineId,
	permission,
}: {
	machineId: string;
	permission: EffectivePermission;
}) {
	const [selectedVolume, setSelectedVolume] = useState<string | null>(null);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const { data: volumes, isPending, error } = useVolumes(machineId);
	const createVolume = useCreateVolume(machineId);
	const deleteVolume = useDeleteVolume(machineId);
	const pruneVolumes = usePruneVolumes(machineId);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [pendingDelete, setPendingDelete] = useState<VolumeSummary | null>(
		null,
	);
	const [exportingName, setExportingName] = useState<string | null>(null);
	const toast = useToast();

	const handleExport = async (volume: VolumeSummary) => {
		setExportingName(volume.name);
		try {
			await downloadFile(
				`/api/machines/${encodeURIComponent(machineId)}/volumes/${encodeURIComponent(volume.name)}/export`,
				`${volume.name}.tar`,
			);
		} catch (error) {
			toast({
				title: `Failed to export volume: ${volume.name}`,
				description: getApiError(error).message,
				status: "error",
				isClosable: true,
			});
		} finally {
			setExportingName(null);
		}
	};

	const canManage = canManageVolumes(permission);

	if (isPending) {
		return <Text data-testid="volumes-loading">Loading volumes…</Text>;
	}

	return (
		<>
			{canManage ? (
				<HStack justify="flex-end" mb={2}>
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => {
							createVolume.reset();
							setIsCreateOpen(true);
						}}
						data-testid="create-volume-button"
					>
						Create volume
					</Button>
					<Button
						size="sm"
						colorScheme="red"
						variant="outline"
						onClick={() => {
							pruneVolumes.reset();
							setIsPruneOpen(true);
						}}
						data-testid="prune-volumes-button"
					>
						Prune
					</Button>
				</HStack>
			) : (
				<Text
					color="gray.500"
					mb={2}
					data-testid="volumes-permission-hint"
					data-permission="read"
				>
					You have read-only access to this machine. Ask an owner or admin for
					write access to manage its volumes.
				</Text>
			)}
			{error ? (
				<ErrorDisplay error={getApiError(error)} data-testid="volumes-error" />
			) : !volumes || volumes.length === 0 ? (
				<Text color="gray.500" data-testid="volumes-empty">
					No volumes on this machine yet.
				</Text>
			) : (
				<TableContainer>
					<Table data-testid="volumes-table" variant="simple">
						<Thead>
							<Tr>
								<Th>Name</Th>
								<Th>Type</Th>
								<Th>Driver</Th>
								<Th>Created</Th>
								<Th isNumeric>Size</Th>
								<Th />
							</Tr>
						</Thead>
						<Tbody>
							{volumes.map((volume) => (
								<VolumeRow
									key={volume.name}
									volume={volume}
									onSelect={setSelectedVolume}
									canManage={canManage}
									onDelete={setPendingDelete}
									isExporting={exportingName === volume.name}
									onExport={handleExport}
								/>
							))}
						</Tbody>
					</Table>
				</TableContainer>
			)}
			<VolumeInspectDrawer
				machineId={machineId}
				volumeName={selectedVolume}
				isOpen={selectedVolume !== null}
				onClose={() => setSelectedVolume(null)}
			/>
			{isPruneOpen ? (
				<PruneVolumesModal
					onClose={() => {
						pruneVolumes.reset();
						setIsPruneOpen(false);
					}}
					pruneVolumes={pruneVolumes}
				/>
			) : null}
			{pendingDelete ? (
				<DeleteVolumeModal
					volume={pendingDelete}
					onClose={() => {
						deleteVolume.reset();
						setPendingDelete(null);
					}}
					deleteVolume={deleteVolume}
				/>
			) : null}
			{isCreateOpen ? (
				<Modal
					isOpen
					onClose={() => {
						createVolume.reset();
						setIsCreateOpen(false);
					}}
					title="Create volume"
				>
					<VolumeForm
						onSubmit={(input) =>
							mutationToPromise(createVolume.mutate, input, {
								onSuccess: (created) => {
									setIsCreateOpen(false);
									toast({
										title: created.anonymous
											? `Anonymous volume created: ${created.name}`
											: `Volume created: ${created.name}`,
										status: "success",
										isClosable: true,
									});
								},
							})
						}
						submitError={createVolume.error}
						isSubmitting={createVolume.isPending}
					/>
				</Modal>
			) : null}
		</>
	);
}
