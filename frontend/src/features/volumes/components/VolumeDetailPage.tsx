import { Badge, Table, TableContainer, Tbody, Text } from "@chakra-ui/react";
import { useVolume } from "@features/volumes/hooks/useVolume";
import type { VolumeDetail } from "@features/volumes/services/volumesApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	isNotFound,
	KeyValueTable,
} from "@shared/components/detailKit";
import { invalidateHostResources } from "@shared/queries/useListInvalidatingMutation";
import { formatBytes, formatRelativeDate } from "@shared/utils/format";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";

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
	const [revealed, setRevealed] = useState(false);
	const { data: volume, isPending, error } = useVolume(hostId!, volumeName!);
	const inspectQuery = useVolume(hostId!, volumeName!, {
		enabled: revealed,
	});

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

	return (
		<DetailPage
			title={volumeName!}
			backTo="volumes"
			isPending={isPending}
			error={error}
			notFound={isNotFound(error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{volume ? <VolumeOverview volume={volume} /> : null}
		</DetailPage>
	);
}
