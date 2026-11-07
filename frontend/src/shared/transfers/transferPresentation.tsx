import {
	Box,
	FormControl,
	FormLabel,
	HStack,
	Radio,
	RadioGroup,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import type { Host } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { formatBytes } from "@shared/utils/format";
import type { ReactNode } from "react";

// The transfer-presentation module (architecture review 2026-10-19,
// candidate C8): the one module owning the two presentation rituals every
// cross-host move/copy surface re-derived — the phase-to-label mapping
// of a transfer's live progress, and the stay-open report's skeleton. The
// adapters stay the wording owners of everything domain (the partial cause
// sentence, the narrations, the error-code hints); the module owns the
// invariants: the phase vocabulary's labels, the header shape, the pinned
// both-hosts tail. The TransferDialog preset's report seam is untouched
// (the issue-241 settlement holds): the skeleton is opt-in presentation the
// caller composes and hands over as the report node.

// The both-hosts tail every partial report ends with: the target install
// stands, so the item exists on both hosts until someone cleans up. A
// user-facing invariant re-derived per feature before this module owned it.
const BOTH_HOSTS_TAIL =
	"It currently exists on both hosts and must be cleaned up manually.";

// The shared phase vocabulary of a transfer's live progress: the phase, the
// optional byte count, the optional skip reason. Both domain progress types
// (the quadlet's per-volume entries and the volume's single entry) are
// structurally assignable, so the adapters pass their entries directly — no
// mapping layer.
export interface TransferProgressEntry {
	phase: "start" | "bytes" | "done" | "skip";
	bytes?: number;
	reason?: string;
}

// One phase-to-label mapping over the shared vocabulary, owned once. A skip
// with a reason renders "skipped — <reason>"; without one, plain "skipped"
// — the quadlet's old copy left a dangling dash, pinned correctly here.
export function transferProgressLabel(progress: TransferProgressEntry): string {
	switch (progress.phase) {
		case "start":
			return "exporting…";
		case "bytes":
			return `${formatBytes(progress.bytes ?? 0)} exported…`;
		case "done":
			return `done (${formatBytes(progress.bytes ?? 0)})`;
		case "skip":
			return progress.reason ? `skipped — ${progress.reason}` : "skipped";
	}
}

// The referencing-quadlet warning every transfer surface renders: the
// quadlets on the source host whose `Secret=`/`Volume=`/... lines resolve to
// the transferred item. They are not transferred, so they keep pointing at
// the source — and after a move those references dangle. The copy is owned
// here because both the single-item dialog and the batch dialog render it
// (spec 486, ticket #487); the adapters pass their noun, the source host's
// name, the preview's quadlet list, and the effective mode.
export interface TransferReferencingWarningProps {
	noun: string;
	hostName: string;
	quadlets: ReadonlyArray<{ name: string }>;
	mode: "move" | "copy";
}

export function TransferReferencingWarning({
	noun,
	hostName,
	quadlets,
	mode,
}: TransferReferencingWarningProps) {
	if (quadlets.length === 0) return null;
	return (
		<Box data-testid="move-referencing-warning">
			<Text>
				Quadlets on this host reference this {noun}. They are not transferred
				and keep pointing at {hostName}:
			</Text>
			<VStack align="flex-start" mt={2} spacing={1}>
				{quadlets.map((quadlet) => (
					<HStack
						key={quadlet.name}
						spacing={2}
						data-testid="move-referencing-row"
					>
						<Text fontFamily="mono">{quadlet.name}</Text>
					</HStack>
				))}
			</VStack>
			{mode === "move" ? (
				<Text mt={2} color="orange.500">
					After a move these references dangle — the {noun} they name is gone
					from this host.
				</Text>
			) : null}
		</Box>
	);
}

// The structural slice of a target-scoped move-preview query the transfer
// dialogs drive. The feature keeps its own preview hook and query key and
// hands over the result; the presentation reads only the states every
// transfer shares — pending, failure, and the collision flag — never the
// domain fields. It lives here because the preview states below render from
// it and both dialogs (single and batch) hand it over; TransferDialog
// re-exports it so the existing import path keeps working.
export interface TransferPreviewSlice {
	isPending: boolean;
	isError: boolean;
	error: unknown;
	data: { collision: boolean } | undefined;
}

// The target picker every transfer dialog renders: the capability-filtered
// host select, or the empty-picker copy when no writable target exists. The
// copy is the adapter's (the quadlet surface keeps its own "you can write
// to" wording), defaulting to the manage-{noun}s phrasing.
export function TransferTargetSelect({
	targets,
	targetId,
	onSelect,
	noun,
	noTargetsMessage,
}: {
	targets: Host[];
	targetId: string;
	onSelect: (targetId: string) => void;
	noun: string;
	noTargetsMessage?: string;
}) {
	return (
		<FormControl isRequired>
			<FormLabel htmlFor="move-target">Target host</FormLabel>
			{targets.length > 0 ? (
				<Select
					id="move-target"
					value={targetId}
					onChange={(event) => onSelect(event.target.value)}
					data-testid="move-target-select"
				>
					{targets.map((candidate) => (
						<option key={candidate.id} value={candidate.id}>
							{candidate.name}
						</option>
					))}
				</Select>
			) : (
				<Text color="gray.500" data-testid="move-no-targets">
					{noTargetsMessage ?? `No other host you can manage ${noun}s on.`}
				</Text>
			)}
		</FormControl>
	);
}

// The move/copy radio, defaulting to Move. A copy-only surface pins the mode
// (the image copy, issue 376): the radio disappears and the dialog's title
// and confirm speak Copy vocabulary only. The two mode radios stack at base
// (spec 393): their labels are too wide to sit side by side at 360px.
export function TransferModeRadio({
	mode,
	onModeChange,
	noun,
	pinnedMode,
}: {
	mode: "move" | "copy";
	onModeChange: (mode: "move" | "copy") => void;
	noun: string;
	pinnedMode?: "copy";
}) {
	if (pinnedMode) return null;
	return (
		<FormControl>
			<FormLabel>Mode</FormLabel>
			<RadioGroup
				value={mode}
				onChange={(next) => onModeChange(next as "move" | "copy")}
				data-testid="move-mode-radio"
			>
				<HStack
					spacing={4}
					flexDirection={{ base: "column", md: "row" }}
					align={{ base: "flex-start", md: "center" }}
				>
					<Radio value="move" data-testid="move-mode-move">
						Move (remove from this host)
					</Radio>
					<Radio value="copy" data-testid="move-mode-copy">
						Copy (keep this host&apos;s {noun})
					</Radio>
				</HStack>
			</RadioGroup>
		</FormControl>
	);
}

// The three states a target-scoped preview narrates: still checking, failed,
// and colliding. The single dialog renders one item's slice, the batch
// dialog one slice per selected item, so the testids are the caller's — each
// surface keeps the vocabulary its own suites assert.
export interface TransferPreviewTestIds {
	pending: string;
	error: string;
	collision: string;
}

export function TransferPreviewStates({
	preview,
	noun,
	itemName,
	targetName,
	testIds,
}: {
	preview: TransferPreviewSlice;
	noun: string;
	itemName: string;
	targetName: string;
	testIds: TransferPreviewTestIds;
}) {
	return (
		<>
			{preview.isPending ? (
				<Text color="gray.500" data-testid={testIds.pending}>
					Checking the target host…
				</Text>
			) : null}
			{preview.isError ? (
				<ErrorDisplay
					error={getApiError(preview.error)}
					data-testid={testIds.error}
				/>
			) : null}
			{preview.data?.collision ? (
				<Text color="red.500" data-testid={testIds.collision}>
					A {noun} named “{itemName}” already exists on {targetName}. Choose
					another target or remove it there first.
				</Text>
			) : null}
		</>
	);
}

// A "partial" transfer envelope is a FAILURE for a batch report (spec 486):
// the target install stands but the source half did not, so the item exists
// on both hosts and its row must stay selected for the retry. The mapping is
// one ritual — the domain sentence plus the envelope's own error line — so
// each adapter keeps only its sentence. Null when the result is not partial.
export function partialOutcome(
	result: { status: string; error?: string },
	sentence: string,
): { ok: false; detail: string } | null {
	if (result.status !== "partial") return null;
	return {
		ok: false,
		detail: `${sentence}${result.error ? ` ${result.error}` : ""}`,
	};
}

// The one status-to-label mapping every transfer narration reads (spec 486
// review): the envelope's own status word, title-cased. A "partial" never
// reaches it from a batch adapter — partialOutcome maps a partial to a
// failure first — but the report header's status union carries it, and it
// narrates like a copy there.
export function transferStatusLabel(
	status: "moved" | "copied" | "partial",
): string {
	return status === "moved" ? "Moved" : "Copied";
}

export interface TransferReportProps {
	// The Moved/Copied header line, with the optional byte-count suffix the
	// volume report carries. Omit it for a partial-only report — the
	// quadlet's partial narrates the both-hosts state without one.
	header?: {
		status: "moved" | "copied" | "partial";
		itemName: string;
		targetName: string;
		bytes?: number;
	};
	// The partial block: the domain's cause sentence (the skeleton appends
	// the pinned both-hosts tail) and the optional red error line.
	partial?: {
		cause: string;
		error?: string;
	};
	// Domain extras — the quadlet's narrations.
	children?: ReactNode;
	// The outer stack's spacing; the quadlet's reports breathe wider.
	spacing?: number;
	// The outer testid, defaulting to today's success report. A headerless
	// partial report passes the partial testid here.
	testid?: string;
	// The partial block's testid when it nests under a header, defaulting to
	// today's partial message.
	partialTestid?: string;
}

// The stay-open report skeleton: the header line, the partial block, and a
// children slot for domain extras, in one shape every move adapter renders
// through. The partial block nests under the header line (the volume and
// secret reports); a headerless report — the quadlet's partial — carries
// the partial content directly, so its outer testid is the partial testid.
export function TransferReport({
	header,
	partial,
	children,
	spacing = 2,
	testid = "move-success-report",
	partialTestid = "move-partial-message",
}: TransferReportProps) {
	const headerLine = header ? (
		<Text>
			{transferStatusLabel(header.status)} “{header.itemName}” to “
			{header.targetName}”
			{header.bytes === undefined
				? "."
				: ` (${formatBytes(header.bytes)} of data).`}
		</Text>
	) : null;
	const partialBody = partial ? (
		<>
			<Text>
				{partial.cause} {BOTH_HOSTS_TAIL}
			</Text>
			{partial.error ? <Text color="red.500">{partial.error}</Text> : null}
		</>
	) : null;
	return (
		<VStack align="flex-start" spacing={spacing} data-testid={testid}>
			{headerLine}
			{partial ? (
				header ? (
					<VStack align="flex-start" spacing={1} data-testid={partialTestid}>
						{partialBody}
					</VStack>
				) : (
					partialBody
				)
			) : null}
			{children}
		</VStack>
	);
}
