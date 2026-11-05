import { Text, VStack } from "@chakra-ui/react";
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
			{header.status === "moved" ? "Moved" : "Copied"} “{header.itemName}” to “
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
