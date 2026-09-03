import {
	Badge,
	Button,
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
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { mutationToPromise } from "@shared/utils/mutationPromise";
import { useState } from "react";
import { VolumeInspectDrawer } from "./VolumeInspectDrawer";

function VolumeRow({
	volume,
	onSelect,
}: {
	volume: VolumeSummary;
	onSelect: (name: string) => void;
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
		</Tr>
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
	const toast = useToast();

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
							</Tr>
						</Thead>
						<Tbody>
							{volumes.map((volume) => (
								<VolumeRow
									key={volume.name}
									volume={volume}
									onSelect={setSelectedVolume}
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
