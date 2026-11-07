import { Text } from "@chakra-ui/react";
import {
	anyVolumeExistsOnTarget,
	hasMigratableVolume,
	isSecretTransferOffered,
	MoveQuadletWarnings,
	moveCompanions,
	moveDifferingCompanions,
	movePreviewSecrets,
} from "@features/quadlets/components/MoveQuadletWarnings";
import {
	useMoveQuadlet,
	useQuadletMovePreviews,
} from "@features/quadlets/hooks/useQuadlets";
import {
	buildMoveInput,
	type QuadletMoveChoices,
	type QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import {
	MultiTransferDialog,
	type MultiTransferItem,
} from "@shared/components/MultiTransferDialog";
import { useTransferState } from "@shared/components/TransferDialog";
import {
	partialOutcome,
	transferStatusLabel,
} from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";

// The batch option set (spec 486, ticket #489): the batch deliberately offers
// no per-item option checkboxes. The ticket's contract is target and mode
// chosen once for the whole selection, and the detail page keeps the full
// single-item option set (stop/force, companions, migration, overwrite,
// secret transfer) for the quadlet that needs it. So the batch fixes the
// safe defaults the single dialog starts from — stop before moving, no
// force, companion .volume definitions ride along, no data migration, no
// overwrite, no secret values — and the payload builder still folds each
// quadlet's own preview facts into its request.
const BATCH_CHOICES: QuadletMoveChoices = {
	stop: true,
	force: false,
	transferCompanions: true,
	migrate: false,
	overwrite: false,
	transferSecrets: false,
};

// The Quadlets batch transfer (spec 486, ticket #489): the shared
// multi-item dialog over the existing per-item move endpoint. The adapter
// keeps the quadlet domain — the manage-quadlets capability the target list
// filters on, the per-quadlet preview queries, the volume/companion/secret
// warnings under each quadlet's name, the plain-mutation transport, and the
// envelope-to-outcome mapping. The both-hosts invalidation stays in the
// mutation hook, where the single-item dialog already leaves it.
//
// The warnings are the single-item dialog's own module, rendered per item
// with `transferCompanions` pinned true and no `acknowledge` prop: the batch
// shows the volume, companion, and secret facts without the stranded-data
// checkbox, because a batch has no per-item acknowledgement to collect (the
// single-item dialog on the detail page keeps it). A differing companion
// therefore surfaces as that item's failure
// in the report rather than blocking the whole batch — the batch has no
// per-item answer to offer, and one quadlet's target conflict must not stop
// the others.
//
// The dialog is opened with a snapshot of the selected quadlets, so the
// report keeps naming them after the list drops the successes from its
// selection.
export function MoveQuadletsDialog({
	host,
	quadlets,
	onFinished,
	onClose,
}: {
	host: Host;
	quadlets: QuadletSummary[];
	onFinished: (succeededKeys: string[]) => void;
	onClose: () => void;
}) {
	const transfer = useTransferState(host, "manage-quadlets");
	const previews = useQuadletMovePreviews(
		host.id,
		quadlets.map((quadlet) => quadlet.id),
		transfer.targetId || null,
	);
	const moveQuadlet = useMoveQuadlet(host.id);

	// One projection per quadlet, computed once and shared by the warnings
	// render and the run: its own preview slice for the dialog's
	// pending/error/collision states, and its own warning lists under its
	// name. The projections are the warnings module's exported functions —
	// the adapter computes none of them inline. The run reads the same
	// facts back by quadlet id, so the request body and the warnings can
	// never disagree about a quadlet's preview.
	const factsById = new Map(
		quadlets.map((quadlet, index) => {
			const preview = previews[index];
			const volumes = preview.data?.volumes ?? [];
			return [
				quadlet.id,
				{
					quadlet,
					preview,
					volumes,
					companions: moveCompanions(volumes),
				},
			];
		}),
	);
	const items: MultiTransferItem<QuadletSummary>[] = quadlets.map((quadlet) => {
		const { preview, volumes, companions } = factsById.get(quadlet.id)!;
		return {
			item: quadlet,
			key: quadlet.id,
			label: quadlet.name,
			preview,
			warnings: (
				<MoveQuadletWarnings
					volumes={volumes}
					differsCompanions={moveDifferingCompanions(companions)}
					secrets={movePreviewSecrets(preview.data)}
					sourceName={host.name}
					targetName={transfer.targetName}
					transferCompanions
				/>
			),
		};
	});

	return (
		<MultiTransferDialog
			host={host}
			transfer={transfer}
			noun="quadlet"
			noTargetsMessage="No other host you can write to."
			items={items}
			runItem={async (quadlet, context) => {
				// The builder is called PER ITEM: it folds this quadlet's own
				// preview facts (companions, migratable, existing target
				// volumes, secret transfer) into the request, so two selected
				// quadlets can send different bodies under one target and mode.
				const { preview, volumes, companions } = factsById.get(quadlet.id)!;
				// `migrate: false` in the batch choices means the builder never
				// selects the NDJSON stream transport, so the batch stays on the
				// plain mutation and needs no progress fold — hence no branch on
				// the builder's `stream` flag here.
				const { input } = buildMoveInput(
					{
						targetHostId: context.targetId,
						companions: companions.length > 0,
						migratable: hasMigratableVolume(volumes),
						anyExistsOnTarget: anyVolumeExistsOnTarget(volumes),
						secretTransferOffered: isSecretTransferOffered(
							host,
							movePreviewSecrets(preview.data),
						),
					},
					context.mode,
					BATCH_CHOICES,
				);
				const result = await moveQuadlet.mutateAsync({
					quadletId: quadlet.id,
					...input,
				});
				// A "partial" answer is a FAILURE for the batch report: the
				// target install stands but the source quadlet could not be
				// removed, so the quadlet still exists on both hosts and the
				// row must stay selected for the retry. The mapping is the
				// shared one; the sentence is this domain's.
				const partial = partialOutcome(
					result,
					`Installed on ${context.targetName} but the source quadlet could not be removed.`,
				);
				if (partial) return partial;
				return {
					ok: true,
					detail: transferStatusLabel(result.status),
				};
			}}
			onFinished={onFinished}
			onClose={onClose}
		>
			{/* The batch offers no per-item option checkboxes, so the fixed
			    stop-before-moving default is stated rather than chosen. */}
			<Text color="gray.500" data-testid="move-batch-stop-note">
				Each selected quadlet is stopped before it moves.
			</Text>
		</MultiTransferDialog>
	);
}
