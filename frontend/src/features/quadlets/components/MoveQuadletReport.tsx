import { HStack, Spinner, Text, VStack } from "@chakra-ui/react";
import type { QuadletMoveVolumeProgress } from "@features/quadlets/hooks/useQuadletMoveStream";
import type { QuadletMoveResult } from "@features/quadlets/services/quadletsApi";
import {
	TransferReport,
	transferProgressLabel,
} from "@shared/transfers/transferPresentation";
import type { ReactNode } from "react";

// The report half of the quadlet transfer adapter (issue 331, architecture
// review 2026-10-19 candidate C6): what a quadlet move narrates, split out
// of MoveQuadletDialog along the seam the adapter already named. The module
// owns the shared outcome-row narration shape, the volume-migration and
// secret-phase narrations, the live per-volume migration progress, and the
// report precedence — a partial result over a completed migration over a
// plain transfer's secret outcomes — as an exported pure projection the
// adapter reuses for its close-on-success effect, so "what narrates" and
// "when the dialog closes" can never drift apart. The stay-open report
// itself renders through the shared transfer-presentation module's skeleton
// (candidate C8): the narrations are the children slot and the partial
// cause sentence is the quadlet domain's, while the header shape and the
// both-hosts tail are the skeleton's. It takes data props only — no
// queries, no hooks; the adapter keeps the move-stream hook.

// Live data-phase progress (issue 214): one line per volume, updated in
// place as its stream advances. The phase-to-label mapping is the shared
// transfer-presentation module's (candidate C8); the entries pass directly.
export function MigrationProgress({
	progress,
}: {
	progress: QuadletMoveVolumeProgress[];
}) {
	return (
		<VStack
			align="flex-start"
			spacing={1}
			data-testid="move-migration-progress"
		>
			<HStack spacing={2}>
				<Spinner size="sm" />
				<Text>Migrating volume data…</Text>
			</HStack>
			{progress.map((entry) => (
				<HStack
					key={entry.podmanVolume}
					spacing={2}
					data-testid={`move-migration-volume-${entry.podmanVolume}`}
				>
					<Text fontFamily="mono">{entry.ref}</Text>
					<Text color="gray.500">{transferProgressLabel(entry)}</Text>
				</HStack>
			))}
		</VStack>
	);
}

// The outcome-row narration shape both phases share (issue 222 review): a
// bold header line, then one row per outcome — a mono-font name and a
// colored result — keyed by testid. The phases differ only in their
// block testid, row testid prefix, entries' identity, display name, and
// result wording, so the shape lives here once and each phase's narration
// is a thin adapter.
function OutcomeNarration<Entry>({
	testid,
	rowTestidPrefix,
	label,
	entries,
	identity,
	name,
	renderOutcome,
}: {
	testid: string;
	rowTestidPrefix: string;
	label: string;
	entries: Entry[];
	identity: (entry: Entry) => string;
	name: (entry: Entry) => string;
	renderOutcome: (entry: Entry) => ReactNode;
}) {
	if (entries.length === 0) return null;
	return (
		<VStack align="flex-start" spacing={1} data-testid={testid}>
			<Text fontWeight="semibold">{label}</Text>
			{entries.map((outcome) => (
				<HStack
					key={identity(outcome)}
					spacing={2}
					data-testid={`${rowTestidPrefix}-${identity(outcome)}`}
				>
					<Text fontFamily="mono">{name(outcome)}</Text>
					{renderOutcome(outcome)}
				</HStack>
			))}
		</VStack>
	);
}

// The data phase's outcomes (issue 214): one line per referenced volume,
// migrated or skipped with its reason — rendered as a successful transfer's
// narration and under a partial message alike.
export function MigrationNarration({ result }: { result: QuadletMoveResult }) {
	return (
		<OutcomeNarration
			testid="move-migration-report"
			rowTestidPrefix="move-migration-outcome"
			label="Volume data:"
			entries={result.volumeMigration?.volumes ?? []}
			identity={(outcome) => outcome.podmanVolume}
			name={(outcome) => outcome.ref}
			renderOutcome={(outcome) =>
				outcome.outcome === "migrated" ? (
					<Text color="green.500">data migrated</Text>
				) : (
					<Text color="gray.500">skipped — {outcome.reason}</Text>
				)
			}
		/>
	);
}

// The secrets phase's outcomes (issue 222): one line per referenced
// secret, transferred or skipped with its reason — rendered beside the
// volume narration wherever the transfer's outcomes are told (a stay-open
// success report and a partial result alike).
export function SecretNarration({ result }: { result: QuadletMoveResult }) {
	return (
		<OutcomeNarration
			testid="move-secret-report"
			rowTestidPrefix="move-secret-outcome"
			label="Secrets:"
			entries={result.secretTransfer?.secrets ?? []}
			identity={(outcome) => outcome.name}
			name={(outcome) => outcome.name}
			renderOutcome={(outcome) =>
				outcome.outcome === "transferred" ? (
					<Text color="green.500">value transferred</Text>
				) : (
					<Text color="gray.500">skipped — {outcome.reason}</Text>
				)
			}
		/>
	);
}

// The report precedence: which result narrates in place, if any — a partial
// result over a completed migration over a plain transfer's secret
// outcomes. A partial result names the both-hosts state and keeps the
// dialog open on either transport; a migrating transfer's completed
// narration and a plain transfer whose answer carries a secretTransfer
// block (issue 222) likewise stay open — the outcomes are the answer's
// point. A file-only copied/moved selects nothing: the adapter's
// close-on-success effect closes on it.
export type MoveQuadletReportSelection =
	| { kind: "partial"; result: QuadletMoveResult }
	| { kind: "migration"; result: QuadletMoveResult }
	| { kind: "secret"; result: QuadletMoveResult };

// The kind names why the dialog stays open, not what the carried result
// is: a plain partial beside a completed stream migration selects kind
// "partial" (the both-hosts state keeps the dialog open) while
// carrying the stream's completed result — its migration outcomes are
// what narrates. Read the kind for the close decision and the result for
// the narration; the two deliberately need not agree.
export function selectMoveQuadletReport(
	plainResult: QuadletMoveResult | null | undefined,
	streamResult: QuadletMoveResult | null | undefined,
): MoveQuadletReportSelection | null {
	if (streamResult?.status === "partial") {
		return { kind: "partial", result: streamResult };
	}
	if (plainResult?.status === "partial") {
		return { kind: "partial", result: streamResult ?? plainResult };
	}
	if (streamResult?.status === "copied" || streamResult?.status === "moved") {
		return { kind: "migration", result: streamResult };
	}
	if (
		plainResult?.secretTransfer &&
		(plainResult.status === "copied" || plainResult.status === "moved")
	) {
		return { kind: "secret", result: plainResult };
	}
	return null;
}

// The stay-open report: a partial result, a migrating transfer's
// outcomes, or a plain transfer's secret outcomes — each narrates in
// place through the shared skeleton (candidate C8); anything else stays
// the form. The narrations are the children slot; the partial cause
// sentence is the quadlet domain's; the header shape, the both-hosts
// tail, and the error line are the skeleton's.
export function MoveQuadletReport({
	selection,
	quadletName,
	sourceName,
	targetName,
}: {
	selection: MoveQuadletReportSelection;
	quadletName: string;
	sourceName: string;
	targetName: string;
}) {
	switch (selection.kind) {
		case "partial":
			return (
				<TransferReport
					testid="move-partial-message"
					spacing={3}
					partial={{
						cause: `The quadlet was installed on “${targetName}” but could not be removed from “${sourceName}”.`,
						error: selection.result.error,
					}}
				>
					<MigrationNarration result={selection.result} />
					<SecretNarration result={selection.result} />
				</TransferReport>
			);
		case "migration":
			return (
				<TransferReport
					testid="move-migration-success"
					spacing={4}
					header={{
						status: selection.result.status,
						itemName: quadletName,
						targetName,
					}}
				>
					<MigrationNarration result={selection.result} />
					<SecretNarration result={selection.result} />
				</TransferReport>
			);
		case "secret":
			return (
				<TransferReport
					testid="move-secret-success"
					spacing={4}
					header={{
						status: selection.result.status,
						itemName: quadletName,
						targetName,
					}}
				>
					<SecretNarration result={selection.result} />
				</TransferReport>
			);
	}
}
