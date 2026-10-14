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

// The wire report mirrors the backend's AutoUpdateReport
// (backend/internal/domain/quadlets/auto_update.go): Containers carries no
// omitempty, so an empty or unparsable report serializes its container list
// as null.
type AutoUpdateReportWire = Omit<AutoUpdateReport, "containers"> & {
	containers: AutoUpdateItem[] | null;
};

interface AutoUpdateEvent {
	operationId: string;
	type: "log" | "complete" | "error";
	stream?: string;
	line?: string;
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
	return (
		typeof event.operationId === "string" &&
		(event.type === "log" ||
			event.type === "complete" ||
			event.type === "error")
	);
}

export interface UseQuadletAutoUpdateResult {
	isRunning: boolean;
	dryRun: boolean;
	// The systemd unit the current or last run was filtered to; null for an
	// unfiltered run.
	filter: string | null;
	lines: AutoUpdateLine[];
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
					if (value.type === "log") {
						if (value.line !== undefined) {
							setLines((current) => [
								...current,
								{ stream: value.stream ?? "stdout", line: value.line! },
							]);
						}
					} else if (value.type === "complete") {
						setReport(normalizeAutoUpdateReport(value.report));
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
		report,
		connectionError,
		run,
		cancel,
	};
}
