import {
	Badge,
	Box,
	Button,
	FormControl,
	FormLabel,
	Td,
	Text,
	Tr,
	useToast,
} from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CreateVolumeDialog } from "@features/volumes/components/CreateVolumeDialog";
import { MoveVolumeDialog } from "@features/volumes/components/MoveVolumeDialog";
import {
	useBulkRemoveVolumes,
	useCreateVolume,
	usePopulateVolume,
	usePruneVolumes,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import {
	buildPopulateForm,
	type VolumeSummary,
	volumeExportUrl,
} from "@features/volumes/services/volumesApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { MutationDialog } from "@shared/components/MutationDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";

import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { downloadFile } from "@shared/utils/download";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The row is a plain function returning the <Tr>, not a component: the
// list's cell-cloning walk (mobile pinning and the selection checkbox
// column) reaches a row's cells only when `renderRow` hands back a `Tr` or a
// `Fragment` of `Tr`s — a component element is opaque to it, so a row
// wrapped in one silently loses both. The key rides the returned `Tr`, which
// is the element the list's map places.
function renderVolumeRow({
	volume,
	onSelect,
	canManage,
	isExporting,
	onExport,
	onPopulate,
	onMove,
}: {
	volume: VolumeSummary;
	onSelect: (name: string) => void;
	canManage: boolean;
	isExporting: boolean;
	onExport: (volume: VolumeSummary) => void;
	onPopulate: (volume: VolumeSummary) => void;
	onMove: (volume: VolumeSummary) => void;
}) {
	return (
		<Tr
			key={volume.name}
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
							aria-label={`Move or copy ${volume.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onMove(volume);
							}}
							data-testid="move-volume-button"
						>
							Move/Copy
						</Button>
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
					</>
				) : null}
			</Td>
		</Tr>
	);
}

const UPLOAD_TOO_LARGE_CODE = "VOLUME_UPLOAD_TOO_LARGE";

// The prune adapter over PruneReportDialog, the twin of the images one:
// the module owns the plumbing, this adapter hands over the noun and the
// two entry accessors that make a volume entry a report row.
function PruneVolumesModal({
	onClose,
	pruneVolumes,
}: {
	onClose: () => void;
	pruneVolumes: ReturnType<typeof usePruneVolumes>;
}) {
	return (
		<PruneReportDialog
			mutation={pruneVolumes}
			resource="volume"
			entryKey={(entry) => entry.name}
			renderLabel={(entry) => entry.name}
			onClose={onClose}
		/>
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

// The Volumes tab body. The toolbar/triad/table plumbing, the plural
// testids, and the hint swap are ResourceList's; this adapter owns only the
// columns, the clickable row navigating to the volume's detail page, and
// the create, export, populate, move/copy, prune, and bulk-remove feature
// code.
//
// Removal has no per-row button (spec 461, ticket #470): a single volume is
// removed from its detail page, several from the toolbar's "Remove
// selected".
export function VolumeList({ host }: { host: Host }) {
	const hostId = host.id;
	const navigate = useNavigate();
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const { data: volumes, isPending, error } = useVolumes(hostId);
	const createVolume = useCreateVolume(hostId);
	const pruneVolumes = usePruneVolumes(hostId);
	const bulkRemoveVolumes = useBulkRemoveVolumes(hostId);
	const populateVolume = usePopulateVolume(hostId);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [populateTarget, setPopulateTarget] = useState<VolumeSummary | null>(
		null,
	);
	const [moveTarget, setMoveTarget] = useState<VolumeSummary | null>(null);
	// The bulk-remove dialog's snapshot: the selected volumes plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		volumes: VolumeSummary[];
		clear: () => void;
	} | null>(null);
	const [exportingName, setExportingName] = useState<string | null>(null);
	const toast = useToast();

	const handleExport = async (volume: VolumeSummary) => {
		setExportingName(volume.name);
		try {
			await downloadFile(
				volumeExportUrl(hostId, volume.name),
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

	const canManage = can(host, "manage-volumes");

	return (
		<>
			<ResourceList
				resource="volume"
				isPending={isPending}
				error={error}
				items={volumes}
				columns={[
					{ label: "Name", isName: true, sortBy: (v) => v.name },
					{
						label: "Type",
						sortBy: (v) => (v.anonymous ? "Anonymous" : "Named"),
					},
					{ label: "Driver", sortBy: (v) => v.driver },
					{ label: "Created", sortBy: (v) => v.createdAt },
					{
						label: "Size",
						isNumeric: true,
						// Unknown sizes ride the key order: last in asc, first in desc.
						sortBy: (v) => v.size ?? Number.POSITIVE_INFINITY,
					},
					{},
				]}
				renderRow={(volume) =>
					renderVolumeRow({
						volume,
						onSelect: (name) => navigate(detailPath(hostId, "volumes", name)),
						canManage,
						isExporting: exportingName === volume.name,
						onExport: handleExport,
						onPopulate: setPopulateTarget,
						onMove: setMoveTarget,
					})
				}
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
							onClick={() => setIsPruneOpen(true)}
							data-testid="prune-volumes-button"
						>
							Prune
						</Button>
					</>
				}
				canManage={canManage}
				selection={{ getKey: (volume) => volume.name }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						volumes: selection.selectedItems,
						clear: selection.clear,
					})
				}
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.volumes}
					noun="volume"
					itemKey={(volume) => volume.name}
					itemLabel={(volume) => volume.name}
					mutation={bulkRemoveVolumes}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
			{populateTarget ? (
				<PopulateVolumeModal
					volume={populateTarget}
					onClose={() => setPopulateTarget(null)}
					populateVolume={populateVolume}
				/>
			) : null}
			{isPruneOpen ? (
				<PruneVolumesModal
					onClose={() => setIsPruneOpen(false)}
					pruneVolumes={pruneVolumes}
				/>
			) : null}
			{moveTarget ? (
				<MoveVolumeDialog
					host={host}
					volume={moveTarget}
					onClose={() => setMoveTarget(null)}
				/>
			) : null}
			{isCreateOpen ? (
				<CreateVolumeDialog
					mutation={createVolume}
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
				/>
			) : null}
		</>
	);
}
