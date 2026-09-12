import {
	Badge,
	Box,
	Button,
	Checkbox,
	FormControl,
	FormLabel,
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
	usePopulateVolume,
	usePruneVolumes,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import {
	buildPopulateForm,
	type VolumePruneReport,
	type VolumeSummary,
} from "@features/volumes/services/volumesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import {
	ConfirmDialog,
	MutationDialog,
} from "@shared/components/MutationDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { VolumeInspectDrawer } from "./VolumeInspectDrawer";

function VolumeRow({
	volume,
	onSelect,
	canManage,
	onDelete,
	isExporting,
	onExport,
	onPopulate,
}: {
	volume: VolumeSummary;
	onSelect: (name: string) => void;
	canManage: boolean;
	onDelete: (volume: VolumeSummary) => void;
	isExporting: boolean;
	onExport: (volume: VolumeSummary) => void;
	onPopulate: (volume: VolumeSummary) => void;
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
					<>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Populate ${volume.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onPopulate(volume);
							}}
						>
							Populate
						</Button>
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
					</>
				) : null}
			</Td>
		</Tr>
	);
}

const VOLUME_IN_USE_CODE = "VOLUME_IN_USE";
const UPLOAD_TOO_LARGE_CODE = "VOLUME_UPLOAD_TOO_LARGE";

// The delete adapter over ConfirmDialog. The module owns the danger footer,
// pending wiring, close-on-success, and the reset lifecycle (on open and on
// close); this adapter keeps the domain body (the force checkbox) and the
// toasts, expressed through the onSuccess/onError seams. The error region is
// composed in renderError: the shared ErrorDisplay always, plus the in-use
// hint when the failure is VOLUME_IN_USE - the seam replaces the region, it
// does not append to it.
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

	return (
		<ConfirmDialog
			mutation={deleteVolume}
			title="Delete volume"
			confirmLabel="Delete volume"
			confirm={() => ({ volumeName: volume.name, force })}
			onClose={onClose}
			onSuccess={() =>
				toast({
					title: `Volume deleted: ${volume.name}`,
					status: "success",
					isClosable: true,
				})
			}
			onError={() =>
				toast({
					title: `Failed to delete volume: ${volume.name}`,
					status: "error",
					isClosable: true,
				})
			}
			body={
				<>
					<Text>
						Delete volume {volume.name} from this machine? This cannot be
						undone.
					</Text>
					<Checkbox
						mt={3}
						isChecked={force}
						onChange={(event) => setForce(event.target.checked)}
						data-testid="force-delete-checkbox"
					>
						Force delete (stops containers using this volume before removal)
					</Checkbox>
				</>
			}
			renderError={(apiError) => (
				<>
					<ErrorDisplay error={apiError} />
					{apiError.code === VOLUME_IN_USE_CODE ? (
						<Text mt={2} color="orange.500" data-testid="volume-in-use-warning">
							Force is required to delete a volume that containers are using.
							Tick force delete and confirm to stop them and remove the volume.
						</Text>
					) : null}
				</>
			)}
		/>
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

// The populate adapter over the core. The body keeps its local state: the
// files input or the tar archive with the shape-conflict guard, and the
// payload building stays a pure function in the API module. The custom
// footer expresses the confirm-populate-button with its progress text while
// pending through the core's footer mechanism. The too-large failure maps
// to its hint through renderError - the seam replaces the whole region, so
// the adapter renders both branches itself; success toasts via onSuccess.
function PopulateVolumeModal({
	volume,
	onClose,
	populateVolume,
}: {
	volume: VolumeSummary;
	onClose: () => void;
	populateVolume: ReturnType<typeof usePopulateVolume>;
}) {
	const [files, setFiles] = useState<FileList | null>(null);
	const [archive, setArchive] = useState<File | null>(null);
	const [progress, setProgress] = useState(0);
	const toast = useToast();

	const hasFiles = files !== null && files.length > 0;
	const hasArchive = archive !== null;
	const shapeConflict = hasFiles && hasArchive;
	const canSubmit = (hasFiles || hasArchive) && !shapeConflict;

	const submit = () => {
		if (!canSubmit) return;
		setProgress(0);
		const form = buildPopulateForm(
			hasFiles ? Array.from(files!) : [],
			hasArchive ? archive : undefined,
		);
		populateVolume.mutate({
			volumeName: volume.name,
			form,
			onProgress: setProgress,
		});
	};

	return (
		<MutationDialog
			mutation={populateVolume}
			title={`Populate ${volume.name}`}
			onClose={onClose}
			onSuccess={() =>
				toast({
					title: `Volume populated: ${volume.name}`,
					status: "success",
					isClosable: true,
				})
			}
			renderError={(apiError) =>
				apiError.code === UPLOAD_TOO_LARGE_CODE ? (
					<>
						<ErrorDisplay error={apiError} />
						<Text
							mt={2}
							color="orange.500"
							data-testid="populate-too-large-hint"
						>
							The upload exceeds the configured size limit. Split or shrink it
							and try again.
						</Text>
					</>
				) : (
					<ErrorDisplay error={apiError} />
				)
			}
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					<Button
						colorScheme="blue"
						onClick={submit}
						isDisabled={!canSubmit}
						isLoading={populateVolume.isPending}
						loadingText={`Populating ${Math.round(progress * 100)}%`}
						data-testid="confirm-populate-button"
					>
						Upload and populate
					</Button>
				</>
			}
			body={
				<>
					<Text>
						Populate merges into the volume: uploaded paths overwrite same-named
						files, everything already in the volume is kept.
					</Text>
					<FormControl mt={3}>
						<FormLabel htmlFor="populate-files">Files</FormLabel>
						<Box data-testid="populate-files-input">
							<input
								id="populate-files"
								type="file"
								multiple
								onChange={(event) => setFiles(event.target.files)}
							/>
						</Box>
					</FormControl>
					<FormControl mt={3}>
						<FormLabel htmlFor="populate-archive">Or a tar archive</FormLabel>
						<Box data-testid="populate-archive-input">
							<input
								id="populate-archive"
								type="file"
								accept=".tar"
								onChange={(event) =>
									setArchive(event.target.files?.[0] ?? null)
								}
							/>
						</Box>
					</FormControl>
					{shapeConflict ? (
						<Text
							color="orange.500"
							mt={2}
							data-testid="populate-shape-conflict"
						>
							Choose either files or a single tar archive, not both.
						</Text>
					) : null}
				</>
			}
		/>
	);
}

// The Volumes tab body. The toolbar/triad/table machinery, the plural
// testids, and the hint swap are ResourceList's; this adapter owns only the
// columns, the clickable row with its inspect drawer, and the create,
// export, populate, delete, and prune feature code.
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
	const populateVolume = usePopulateVolume(machineId);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [populateTarget, setPopulateTarget] = useState<VolumeSummary | null>(
		null,
	);
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

	return (
		<>
			<ResourceList
				resource="volume"
				isPending={isPending}
				error={error}
				items={volumes}
				columns={[
					{ label: "Name" },
					{ label: "Type" },
					{ label: "Driver" },
					{ label: "Created" },
					{ label: "Size", isNumeric: true },
					{},
				]}
				renderRow={(volume) => (
					<VolumeRow
						key={volume.name}
						volume={volume}
						onSelect={setSelectedVolume}
						canManage={canManage}
						onDelete={setPendingDelete}
						isExporting={exportingName === volume.name}
						onExport={handleExport}
						onPopulate={setPopulateTarget}
					/>
				)}
				toolbar={
					<>
						<Button
							size="sm"
							colorScheme="blue"
							onClick={() => setIsCreateOpen(true)}
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
					</>
				}
				canManage={canManage}
			/>
			<VolumeInspectDrawer
				machineId={machineId}
				volumeName={selectedVolume}
				isOpen={selectedVolume !== null}
				onClose={() => setSelectedVolume(null)}
			/>
			{populateTarget ? (
				<PopulateVolumeModal
					volume={populateTarget}
					onClose={() => setPopulateTarget(null)}
					populateVolume={populateVolume}
				/>
			) : null}
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
					onClose={() => setPendingDelete(null)}
					deleteVolume={deleteVolume}
				/>
			) : null}
			{isCreateOpen ? (
				<MutationDialog
					mutation={createVolume}
					title="Create volume"
					onClose={() => setIsCreateOpen(false)}
					// The created volume arrives through the live mutation result by
					// the time the module's onSuccess seam fires.
					onSuccess={() => {
						const created = createVolume.data;
						if (created) {
							toast({
								title: created.anonymous
									? `Anonymous volume created: ${created.name}`
									: `Volume created: ${created.name}`,
								status: "success",
								isClosable: true,
							});
						}
					}}
					body={
						<VolumeForm
							onSubmit={(input) => createVolume.mutate(input)}
							isSubmitting={createVolume.isPending}
						/>
					}
				/>
			) : null}
		</>
	);
}
