import { Badge, Button, Td, Tr, useToast } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CreateVolumeDialog } from "@features/volumes/components/CreateVolumeDialog";
import { MoveVolumesDialog } from "@features/volumes/components/MoveVolumesDialog";
import { PopulateVolumeDialog } from "@features/volumes/components/PopulateVolumeDialog";
import { VolumeActionButtons } from "@features/volumes/components/VolumeActionButtons";
import { useVolumeExport } from "@features/volumes/hooks/useVolumeExport";
import {
	useBulkRemoveVolumes,
	useCreateVolume,
	usePopulateVolume,
	usePruneVolumes,
	useVolumes,
} from "@features/volumes/hooks/useVolumes";
import type { VolumeSummary } from "@features/volumes/services/volumesApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";

import {
	ResourceList,
	type ResourceListSelectionSnapshot,
	snapshotSelection,
} from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The row is a plain function returning the <Tr>, not a component: the
// list's cell-cloning walk (mobile pinning and the selection checkbox
// column) reaches a row's cells only when `renderRow` hands back a `Tr` or a
// `Fragment` of `Tr`s — a component element is opaque to it, so a row
// wrapped in one silently loses both. The key rides the returned `Tr`, which
// is the element the list's map places.
//
// The action cell is the shared action set's (spec #477), which the volume
// detail page renders too: Export reads at read level, Populate rides the
// manage-volumes gate. The row carries no Move/Copy button (spec #486,
// ticket #490): moving is the toolbar's "Move/copy selected", which serves
// one selected row and many alike, so the action set is handed no `onMove`.
function renderVolumeRow({
	volume,
	onSelect,
	canManage,
	isExporting,
	onExport,
	onPopulate,
}: {
	volume: VolumeSummary;
	onSelect: (name: string) => void;
	canManage: boolean;
	isExporting: boolean;
	onExport: (volume: VolumeSummary) => void;
	onPopulate: (volume: VolumeSummary) => void;
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
				<VolumeActionButtons
					volume={volume}
					canManage={canManage}
					isExporting={isExporting}
					onExport={() => onExport(volume)}
					onPopulate={() => onPopulate(volume)}
				/>
			</Td>
		</Tr>
	);
}

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

// The Volumes tab body. The toolbar/triad/table plumbing, the plural
// testids, and the hint swap are ResourceList's; this adapter owns only the
// columns, the clickable row navigating to the volume's detail page, and
// the create, export, populate, batch move/copy, prune, and bulk-remove
// feature code.
//
// Removal has no per-row button (spec 461, ticket #470): a single volume is
// removed from its detail page, several from the toolbar's "Remove
// selected". Move/copy follows the same shape (spec #486, ticket #490): the
// row's per-volume button is gone, and the toolbar's "Move/copy selected"
// serves one selected row and many alike — one target and mode chosen once
// for the selection, then a per-item report. Succeeded volumes leave the
// selection; failed ones stay for the retry.
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
	// The bulk-remove dialog's snapshot: the selected volumes plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		volumes: VolumeSummary[];
		clear: () => void;
	} | null>(null);
	// The batch transfer's snapshot, the same shape: the selected volumes
	// plus the selection's partial reset. A transfer clears only the volumes
	// that succeeded, so the failures stay selected for the retry.
	const [moveSelectedTarget, setMoveSelectedTarget] =
		useState<ResourceListSelectionSnapshot<VolumeSummary> | null>(null);
	const toast = useToast();
	// The export ritual is the shared hook's (spec #477), so the detail
	// page's Export button runs the identical one: a per-volume busy state,
	// the volume's own export endpoint, and an error toast naming it.
	const { exportingName, exportVolume } = useVolumeExport(hostId);

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
						onExport: (target) => exportVolume(target.name),
						onPopulate: setPopulateTarget,
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
				// The batch transfer sits beside Remove selected and is
				// permanently visible for a manage-volumes caller — the
				// capability the row button it replaces carried — disabling
				// itself while nothing is selected. The module renders it for
				// a write-capable caller only, so a read-only caller keeps the
				// permission-hint swap untouched.
				selectionActions={
					canManage
						? (selection) => (
								<Button
									size="sm"
									variant="outline"
									isDisabled={selection.selectedKeys.length === 0}
									onClick={() =>
										setMoveSelectedTarget(snapshotSelection(selection))
									}
									data-testid="move-selected-volumes-button"
								>
									Move/copy selected
								</Button>
							)
						: undefined
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
			{moveSelectedTarget ? (
				<MoveVolumesDialog
					host={host}
					volumes={moveSelectedTarget.items}
					// Only the succeeded volumes leave the selection.
					onFinished={moveSelectedTarget.clearKeys}
					onClose={() => setMoveSelectedTarget(null)}
				/>
			) : null}
			{populateTarget ? (
				<PopulateVolumeDialog
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
