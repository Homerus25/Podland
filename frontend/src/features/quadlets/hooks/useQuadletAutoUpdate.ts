import { autoUpdateStreamUrl } from "@features/quadlets/services/quadletsApi";
import { invalidateMachineResources } from "@shared/queries/useListInvalidatingMutation";
import { useNdjsonOperation } from "@shared/streaming/streamTransport";
import type { ApiError } from "@shared/types/api";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

// Event and report shapes mirror the backend's AutoUpdateEvent
// (backend/internal/domain/quadlets/auto_update.go).
export interface AutoUpdateItem {
	unit?: string;
	containerName: string;
	imageName: string;
	// The backend's outcome vocabulary: "updated" | "failed" | "pending" | "up-to-date".
	updated: string;
	error?: string;
}

export interface AutoUpdateReport {
	// Always an array by the time it reaches state: the hook normalizes
	// the wire's null to [] at the event seam (see AutoUpdateReportWire).
	containers: AutoUpdateItem[];
	overall: string;
	exitCode: number;
}

export interface AutoUpdateLine {
	stream: string;
	line: string;
}

// One quadlet's live state in a run, folded from the stream's plan, pulling,
// and report events the way the move stream folds per-volume events: the
// plan event fixes the rows and their order (first-seen), a pulling event
// flips its row in place (latest state wins), and the report resolves rows
// by unit. A unit absent from the plan but present in the report is appended
// as an already-resolved row; a planned unit the report never mentions keeps
// its last observed state — there is no outcome to show for it.
export type AutoUpdateRowState =
	| "waiting"
	| "pulling"
	| "updated"
	| "failed"
	| "pending"
	| "up-to-date";

export interface AutoUpdateRow {
	// The systemd unit the row tracks; null on a report-resolved row whose
	// report item carried no unit.
	unit: string | null;
	// The plan entry's image — absent on a degraded entry (unreadable file
	// or unparsable Image=), so such a row can never be matched mid-run.
	image?: string;
	state: AutoUpdateRowState;
	// The report item that resolved the row; present only after the
	// completing report.
	item?: AutoUpdateItem;
}

// A pulling event flips its row in place (latest state wins); a unit the
// plan did not name is appended as a pulling row, the move stream's
// first-seen rule.
function foldPulling(unit: string, current: AutoUpdateRow[]): AutoUpdateRow[] {
	const index = current.findIndex((row) => row.unit === unit);
	if (index === -1) return [...current, { unit, state: "pulling" }];
	return current.map((row, at) =>
		at === index ? { ...row, state: "pulling" } : row,
	);
}

// The report's outcome vocabulary is the backend's closed set of
// AutoUpdateItem constants; a state outside it is malformed content, and
// the fold skips resolving that row (the hook's warn-and-skip guard),
// keeping the row's last observed state.
const autoUpdateRowStates: ReadonlySet<string> = new Set([
	"updated",
	"failed",
	"pending",
	"up-to-date",
]);

// The completing report resolves rows by unit: a report item naming a
// planned unit resolves that row in place; an item no row tracks (no unit,
// or a unit absent from the plan) is appended as an already-resolved row.
// A planned unit the report never mentions keeps its last observed state.
function foldReportItems(
	report: AutoUpdateReport,
	current: AutoUpdateRow[],
): AutoUpdateRow[] {
	const rows = [...current];
	for (const item of report.containers) {
		// The wire type keeps the state a plain string, so it narrows
		// here — and an unknown state skips the item rather than folding
		// a nonsense state onto the row.
		if (!autoUpdateRowStates.has(item.updated)) {
			console.warn(
				"Skipping report item with unknown state (malformed content):",
				item,
			);
			continue;
		}
		const state = item.updated as AutoUpdateRowState;
		const index = item.unit
			? rows.findIndex((row) => row.unit === item.unit)
			: -1;
		if (index === -1) {
			rows.push({
				unit: item.unit ?? null,
				image: item.imageName,
				state,
				item,
			});
		} else {
			rows[index] = { ...rows[index], state, item };
		}
	}
	return rows;
}

// The wire plan entry mirrors the backend's AutoUpdatePlanItem
// (backend/internal/domain/quadlets/auto_update.go): a unit name plus its
// image, the image absent on degraded entries.
interface AutoUpdatePlanItemWire {
	unit: string;
	image?: string;
}

// The wire report mirrors the backend's AutoUpdateReport
// (backend/internal/domain/quadlets/auto_update.go): Containers carries no
// omitempty, so an empty or unparsable report serializes its container list
// as null.
type AutoUpdateReportWire = Omit<AutoUpdateReport, "containers"> & {
	containers: AutoUpdateItem[] | null;
};

interface AutoUpdateEvent {
	operationId: string;
	type: "plan" | "pulling" | "log" | "complete" | "error";
	stream?: string;
	line?: string;
	plan?: AutoUpdatePlanItemWire[];
	unit?: string;
	image?: string;
	report?: AutoUpdateReportWire | null;
	error?: { code: string; message: string };
}

// The backend serializes an empty report's container list as null (a Go
// nil slice without omitempty), so an unparsable or empty report arrives
// with containers: null. The hook owns the report shape and normalizes it
// here, so the modal never dereferences null.
function normalizeAutoUpdateReport(
	report: AutoUpdateReportWire | null | undefined,
): AutoUpdateReport | null {
	if (!report) return null;
	return { ...report, containers: report.containers ?? [] };
}

function isAutoUpdateEvent(value: unknown): value is AutoUpdateEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	if (typeof event.operationId !== "string") return false;
	if (event.type === "plan") return Array.isArray(event.plan);
	if (event.type === "pulling") return typeof event.unit === "string";
	return (
		event.type === "log" || event.type === "complete" || event.type === "error"
	);
}

// The plan event seeds the row list: one waiting row per plan entry, in
// plan order. Entries without a usable unit name are skipped — a row needs
// a unit to be matched by pulling events and the report.
function planRows(plan: AutoUpdatePlanItemWire[]): AutoUpdateRow[] {
	return plan
		.filter(
			(entry): entry is AutoUpdatePlanItemWire =>
				typeof entry === "object" &&
				entry !== null &&
				typeof (entry as AutoUpdatePlanItemWire).unit === "string" &&
				(entry as AutoUpdatePlanItemWire).unit !== "",
		)
		.map((entry) => ({
			unit: entry.unit,
			...(entry.image !== undefined ? { image: entry.image } : {}),
			state: "waiting" as const,
		}));
}

export interface UseQuadletAutoUpdateResult {
	isRunning: boolean;
	dryRun: boolean;
	// The systemd unit the current or last run was filtered to; null for an
	// unfiltered run.
	filter: string | null;
	lines: AutoUpdateLine[];
	// The live per-quadlet status list, folded from the plan, pulling, and
	// report events (see AutoUpdateRow). Empty until the plan event arrives.
	rows: AutoUpdateRow[];
	// The unit named by the most recent pulling event — the run's current
	// activity, null before the first pulling event. Rows stay "pulling"
	// until the report resolves them (podman reports no per-unit completion
	// mid-run), so this is the only event-ordered signal.
	pullingUnit: string | null;
	report: AutoUpdateReport | null;
	// A terminal error event (machine unreachable, SSH failure) or an HTTP
	// failure — distinct from per-container failures inside the report.
	connectionError: ApiError | null;
	run: (dryRun: boolean, filter?: string) => void;
	cancel: () => void;
}

// The auto-update run rides the shared stream transport; its "log" event
// vocabulary stays feature-side with the local shape guard, as do the
// accumulated lines, the completion report, and the dry-run-aware
// invalidation.
export function useQuadletAutoUpdate(
	machineId: string,
): UseQuadletAutoUpdateResult {
	const queryClient = useQueryClient();
	const [dryRun, setDryRun] = useState(false);
	const [filter, setFilter] = useState<string | null>(null);
	const [lines, setLines] = useState<AutoUpdateLine[]>([]);
	const [rows, setRows] = useState<AutoUpdateRow[]>([]);
	const [pullingUnit, setPullingUnit] = useState<string | null>(null);
	const [report, setReport] = useState<AutoUpdateReport | null>(null);
	const {
		isRunning,
		error: connectionError,
		start,
		cancel,
	} = useNdjsonOperation();

	const run = useCallback(
		(runDryRun: boolean, runFilter?: string) => {
			setDryRun(runDryRun);
			setFilter(runFilter ?? null);
			setLines([]);
			setRows([]);
			setPullingUnit(null);
			setReport(null);
			start({
				url: autoUpdateStreamUrl(machineId, {
					dryRun: runDryRun,
					filter: runFilter,
				}),
				init: { method: "POST" },
				onEvent: (value) => {
					if (!isAutoUpdateEvent(value)) {
						console.warn(
							"Skipping malformed stream event (unexpected shape):",
							value,
						);
						return;
					}
					if (value.type === "plan") {
						setRows(planRows(value.plan ?? []));
					} else if (value.type === "pulling") {
						setRows((current) => foldPulling(value.unit ?? "", current));
						setPullingUnit(value.unit ?? null);
					} else if (value.type === "log") {
						if (value.line !== undefined) {
							setLines((current) => [
								...current,
								{ stream: value.stream ?? "stdout", line: value.line! },
							]);
						}
					} else if (value.type === "complete") {
						const normalized = normalizeAutoUpdateReport(value.report);
						setReport(normalized);
						if (normalized) {
							setRows((current) => foldReportItems(normalized, current));
						}
						// Restarted containers and pulled images only change on a real
						// run; a dry run mutates nothing, so its queries stay fresh.
						if (!runDryRun) {
							invalidateMachineResources(queryClient, machineId, [
								"quadlets",
								"containers",
								"images",
							]);
						}
					}
				},
			});
		},
		[machineId, queryClient, start],
	);

	return {
		isRunning,
		dryRun,
		filter,
		lines,
		rows,
		pullingUnit,
		report,
		connectionError,
		run,
		cancel,
	};
}
