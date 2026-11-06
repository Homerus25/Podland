import { Badge, Table, TableContainer, Tbody, Text } from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { useHost } from "@features/hosts/hooks/useHosts";
import { MoveVolumeDialog } from "@features/volumes/components/MoveVolumeDialog";
import { PopulateVolumeDialog } from "@features/volumes/components/PopulateVolumeDialog";
import { VolumeActionButtons } from "@features/volumes/components/VolumeActionButtons";
import { VOLUME_IN_USE_REFUSAL } from "@features/volumes/components/volumeRemoval";
import { useVolume } from "@features/volumes/hooks/useVolume";
import { useVolumeExport } from "@features/volumes/hooks/useVolumeExport";
import {
	useDeleteVolume,
	usePopulateVolume,
} from "@features/volumes/hooks/useVolumes";
import type { VolumeDetail } from "@features/volumes/services/volumesApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	headerActionGroup,
	isNotFound,
	KeyValueTable,
	removalAction,
} from "@shared/components/detailKit";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { invalidateHostResources } from "@shared/queries/useListInvalidatingMutation";
import { hostTabPath } from "@shared/routes/detailRoute";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

// The volume's overview rows, adapted from the former VolumeInspectDrawer:
// the same label/value table, with the key-value tables for labels/options.
function VolumeOverview({ volume }: { volume: VolumeDetail }) {
	return (
		<TableContainer data-testid="volume-detail-overview">
			<Table size="sm">
				<Tbody>
					<DetailRow label="Name">
						<Text fontFamily="mono">{volume.name}</Text>
					</DetailRow>
					<DetailRow label="Driver">{volume.driver}</DetailRow>
					<DetailRow label="Type">
						<Badge
							colorScheme={volume.anonymous ? "gray" : "purple"}
							data-testid="volume-detail-type-badge"
						>
							{volume.anonymous ? "Anonymous" : "Named"}
						</Badge>
					</DetailRow>
					<DetailRow label="Created">
						<Text data-testid="volume-detail-created">
							{formatRelativeDate(volume.createdAt)}
						</Text>
					</DetailRow>
					<DetailRow label="Size">
						{volume.size !== undefined ? formatBytes(volume.size) : "—"}
					</DetailRow>
					<DetailRow label="Mountpoint">
						<Text fontFamily="mono" wordBreak="break-all">
							{volume.mountpoint}
						</Text>
					</DetailRow>
				</Tbody>
			</Table>
			<KeyValueTable
				label="Labels"
				testId="volume-detail-labels"
				values={volume.labels}
			/>
			<KeyValueTable
				label="Options"
				testId="volume-detail-options"
				values={volume.options}
			/>
		</TableContainer>
	);
}

// The Remove action's confirm (spec 461, ticket #465): the shared
// force-refusal preset, so the first confirm sends the plain delete and
// only the VOLUME_IN_USE refusal swaps in the in-use hint and the force
// button whose retry carries force=true. The adapter keeps the domain
// vocabulary only — the refusal words come from the volumeRemoval module
// the list's modal reads too, and the success side effect (navigate back
// to the list tab) arrives through the preset's onSuccess seam.
function RemoveVolumeDialog({
	volumeName,
	onClose,
	deleteVolume,
	onSuccess,
}: {
	volumeName: string;
	onClose: () => void;
	deleteVolume: ReturnType<typeof useDeleteVolume>;
	onSuccess: () => void;
}) {
	return (
		<ForceConfirmDialog
			mutation={deleteVolume}
			title="Remove volume"
			onClose={onClose}
			refusalCode={VOLUME_IN_USE_REFUSAL.code}
			hint={VOLUME_IN_USE_REFUSAL.hint}
			hintTestId={VOLUME_IN_USE_REFUSAL.hintTestId}
			confirmLabel="Remove volume"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ volumeName, force })}
			onSuccess={onSuccess}
			body={
				<Text>
					Remove volume {volumeName} from this host? This cannot be undone.
				</Text>
			}
		/>
	);
}

// The volume detail page (spec 301, ticket #307): the shared DetailPage
// layout over the volume's inspect response. The overview query is the
// volume detail (GET .../volumes/{name}); the "Show inspect JSON" reveal
// rides the same hook with enabled: revealed — volumes have no raw /inspect
// endpoint, so the typed DTO serialized as JSON is the inspect report.
export function VolumeDetailPage() {
	const { hostId, volumeName } = useParams<{
		hostId: string;
		volumeName: string;
	}>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [revealed, setRevealed] = useState(false);
	// The remove confirm (spec 461, ticket #465): rendered conditionally, so
	// mount is open and unmount is close.
	const [removeOpen, setRemoveOpen] = useState(false);
	// The move/copy transfer and the populate upload (spec 477, ticket #482):
	// rendered conditionally, so mount is open and unmount is close.
	const [moving, setMoving] = useState(false);
	const [populating, setPopulating] = useState(false);
	const { data: volume, isPending, error } = useVolume(hostId!, volumeName!);
	const inspectQuery = useVolume(hostId!, volumeName!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "manage-volumes");
	// The transfer dialog needs the whole host, not just its id: the shared
	// transfer state filters the target list by capability and excludes the
	// source. The gate's hook rides the same host query, so this adds no
	// request.
	const host = useHost(hostId!);
	const deleteVolume = useDeleteVolume(hostId!);
	// The page owns its own populate mutation: the dialog takes it as a prop
	// because the list keeps one instance for every row. It invalidates the
	// volumes list, whose key this page's detail query extends, so the
	// overview refetches on success.
	const populateVolume = usePopulateVolume(hostId!);
	const notFound = isNotFound(error);

	// Export is a read, so it renders for every caller. The ritual — the busy
	// state, the volume's own export endpoint, the error toast — is the
	// shared hook's, so the list row's Export button runs the identical one.
	const { exportingName, exportVolume } = useVolumeExport(hostId!);

	// A 404 means the row was stale (the volume was deleted since the list
	// loaded); refetch the list so the dead row disappears. The list key is
	// a prefix of this query's key, so the invalidation refetches this query
	// too; each refetched 404 is a fresh error object, which would re-trigger
	// this effect forever. One invalidation per stale volume is what the flow
	// needs.
	const invalidatedRef = useRef<string | null>(null);
	useEffect(() => {
		if (isNotFound(error) && invalidatedRef.current !== volumeName) {
			invalidatedRef.current = volumeName!;
			invalidateHostResources(queryClient, hostId!, "volumes");
		}
	}, [volumeName, error, queryClient, hostId]);

	// The Remove action rides the detail-page capability gate (the kit owns
	// the cascade): allowed renders the button, denied the shared write hint,
	// pending neither, and a not-found overview hides it outright.
	const removeAction = removalAction({
		gate,
		notFound,
		scope: "volumes",
		resource: "volume",
		testId: "remove-volume-button",
		onClick: () => setRemoveOpen(true),
	});

	// The header's action row (spec #477), composed through the detail kit's
	// one owner: the row's three buttons beside Remove. Export reads at read
	// level, so it is ungated and renders for every caller; Move/Copy and
	// Populate ride the manage-volumes gate and pass no hint, because
	// Remove's cascade already renders the page's write hint for the same
	// capability (the one-hint rule). The control self-gates on the volume
	// DTO, since the header renders while the overview query is still in
	// flight — and a not-found page has no volume to act on.
	const headerActions = headerActionGroup({
		actions: [
			{
				control: volume ? (
					<VolumeActionButtons
						volume={volume}
						canManage={gate === "allowed"}
						isExporting={exportingName === volume.name}
						size="sm"
						onExport={() => exportVolume(volume.name)}
						onMove={() => setMoving(true)}
						onPopulate={() => setPopulating(true)}
					/>
				) : null,
			},
		],
		remove: removeAction,
	});

	return (
		<>
			<DetailPage
				title={volumeName!}
				backTo="volumes"
				isPending={isPending}
				error={error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={headerActions}
			>
				{volume ? <VolumeOverview volume={volume} /> : null}
			</DetailPage>
			{moving && volume && host.data ? (
				<MoveVolumeDialog
					host={host.data}
					volume={volume}
					onClose={() => setMoving(false)}
				/>
			) : null}
			{populating && volume ? (
				<PopulateVolumeDialog
					volume={volume}
					onClose={() => setPopulating(false)}
					populateVolume={populateVolume}
				/>
			) : null}
			{removeOpen ? (
				<RemoveVolumeDialog
					volumeName={volumeName!}
					onClose={() => setRemoveOpen(false)}
					deleteVolume={deleteVolume}
					// The removed volume's page would 404 on the next refetch, so
					// the page leaves for the list tab; the mutation's own
					// invalidation refetches that list.
					onSuccess={() => navigate(hostTabPath(hostId!, "volumes"))}
				/>
			) : null}
		</>
	);
}
