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
	containers: AutoUpdateItem[];
	overall: string;
	exitCode: number;
}

export interface AutoUpdateLine {
	stream: string;
	line: string;
}

interface AutoUpdateEvent {
	operationId: string;
	type: "log" | "complete" | "error";
	stream?: string;
	line?: string;
	report?: AutoUpdateReport;
	error?: { code: string; message: string };
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
						setReport(value.report ?? null);
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
