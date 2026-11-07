import { Box, Text } from "@chakra-ui/react";
import {
	type VolumeMoveProgress,
	type VolumeMoveStreamEvent,
	volumeMoveStreamConfig,
} from "@features/volumes/hooks/useVolumeMoveStream";
import { useVolumeMovePreviews } from "@features/volumes/hooks/useVolumes";
import type {
	VolumeMoveResult,
	VolumeSummary,
} from "@features/volumes/services/volumesApi";
import {
	MultiTransferDialog,
	type MultiTransferItem,
} from "@shared/components/MultiTransferDialog";
import { useTransferState } from "@shared/components/TransferDialog";
import {
	runMoveStream,
	transferErrorMessage,
} from "@shared/transfers/transferHooks";
import {
	partialOutcome,
	TransferReferencingWarning,
	transferProgressLabel,
	transferStatusLabel,
} from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";
import { formatBytes } from "@shared/utils/format";
import { useQueryClient } from "@tanstack/react-query";

// The Volumes batch transfer (spec 486, ticket #490): the shared multi-item
// dialog over the existing per-item move endpoint. The adapter keeps the
// volume domain — the manage-volumes capability the target list filters on,
// the per-volume preview queries, the referencing-quadlet and in-use
// warnings, the NDJSON transport (the endpoint always streams, so each item
// runs through the same stream config the single-item dialog uses), and the
// envelope-to-outcome mapping. The both-hosts invalidation stays in the
// stream runner, where the single-item dialog already leaves it.
//
// The batch sends no force: a move that refuses because a container uses the
// volume is reported as a failed item, and the volume's own page keeps the
// single-item force path. The dialog is opened with a snapshot of the
// selected volumes, so the report keeps naming them after the list drops the
// successes from its selection.
export function MoveVolumesDialog({
	host,
	volumes,
	onFinished,
	onClose,
}: {
	host: Host;
	volumes: VolumeSummary[];
	onFinished: (succeededKeys: string[]) => void;
	onClose: () => void;
}) {
	const transfer = useTransferState(host, "manage-volumes");
	const previews = useVolumeMovePreviews(
		host.id,
		volumes.map((volume) => volume.name),
		transfer.targetId || null,
	);
	const queryClient = useQueryClient();
	// The stream config is the single-item hook's, so the batch and the
	// volume's own page cannot drift on the event vocabulary, the shape
	// guard, or the progress fold.
	const streamConfig = volumeMoveStreamConfig(host.id);

	const items: MultiTransferItem<VolumeSummary>[] = volumes.map(
		(volume, index) => {
			const preview = previews[index];
			return {
				item: volume,
				key: volume.name,
				label: volume.name,
				preview,
				// The same referencing-quadlet warning the single-item dialog
				// renders, under this volume's own name, plus the in-use note
				// the batch can only narrate: it sends no force, so the way out
				// is copy mode or the volume's own page.
				warnings: (
					<>
						<TransferReferencingWarning
							noun="volume"
							hostName={host.name}
							quadlets={preview.data?.referencingQuadlets ?? []}
							mode={transfer.mode}
						/>
						{transfer.mode === "move" && preview.data?.inUse ? (
							<Box data-testid="move-inuse-warning">
								<Text color="orange.500">
									A container on this host is using this volume. The move
									refuses while it is in use — copy it instead, or move it from
									the volume&apos;s own page with force.
								</Text>
							</Box>
						) : null}
					</>
				),
			};
		},
	);

	return (
		<MultiTransferDialog
			host={host}
			transfer={transfer}
			noun="volume"
			items={items}
			runItem={async (volume, context) => {
				let result: VolumeMoveResult;
				try {
					result = await runMoveStream<
						VolumeMoveStreamEvent,
						VolumeMoveProgress | null,
						VolumeMoveResult
					>(
						streamConfig,
						queryClient,
						volume.name,
						{ targetHostId: context.targetId, mode: context.mode },
						{
							signal: context.signal,
							onProgress: (progress) => {
								if (progress) {
									context.onProgress(transferProgressLabel(progress));
								}
							},
						},
					);
				} catch (error) {
					// A refused item is a failed report row, not a failed batch:
					// the run continues with the next volume.
					return { ok: false, detail: transferErrorMessage(error) };
				}
				// A "partial" answer is a FAILURE for the batch report: the data
				// crossed but the source volume could not be removed, so the
				// volume still exists on both hosts and the row must stay
				// selected for the retry. The mapping is the shared one; the
				// sentence is this domain's.
				const partial = partialOutcome(
					result,
					"The data crossed but the source volume could not be removed.",
				);
				if (partial) return partial;
				return {
					ok: true,
					detail: `${transferStatusLabel(result.status)} (${formatBytes(result.bytes)})`,
				};
			}}
			onFinished={onFinished}
			onClose={onClose}
		/>
	);
}
