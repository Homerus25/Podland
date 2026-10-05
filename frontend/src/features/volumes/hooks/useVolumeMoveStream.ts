import { invalidateVolumeMoveResult } from "@features/volumes/hooks/useVolumes";
import type {
	VolumeMoveInput,
	VolumeMoveResult,
} from "@features/volumes/services/volumesApi";
import { volumeMoveStreamUrl } from "@features/volumes/services/volumesApi";
import { useNdjsonOperation } from "@shared/streaming/streamTransport";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

// The transfer's live data-phase state, folded from the stream's progress
// events (backend/internal/domain/volumes/dto.go VolumeMoveEvent): one
// volume, the latest phase winning. The backend emits a "bytes" event per
// MiB crossed, so the entry updates in place instead of accumulating.
export interface VolumeMoveProgress {
	phase: "start" | "bytes" | "done";
	bytes?: number;
}

// Event shape mirror of the backend's VolumeMoveEvent (issue 238): progress
// events for the one volume, then exactly one terminal event — the transfer
// envelope on "complete", the mapped code/message on "error".
interface VolumeMoveStreamEvent {
	operationId: string;
	type: "volume" | "complete" | "error";
	machineId: string;
	volume?: string;
	phase?: VolumeMoveProgress["phase"];
	bytes?: number;
	result?: VolumeMoveResult;
	error?: { code: string; message: string };
}

function isVolumeMoveStreamEvent(
	value: unknown,
): value is VolumeMoveStreamEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	return (
		typeof event.operationId === "string" &&
		(event.type === "volume" ||
			event.type === "complete" ||
			event.type === "error")
	);
}

export interface UseVolumeMoveStreamResult {
	isRunning: boolean;
	// The data phase's latest state. Null until the first progress event
	// arrives and once the transfer has settled.
	progress: VolumeMoveProgress | null;
	// The transfer envelope from the terminal complete event. Null until
	// then and on any failure.
	result: VolumeMoveResult | null;
	// A terminal error event (e.g. VOLUME_ALREADY_EXISTS) or a transport
	// failure — never an abort.
	error: { code: string; message: string } | null;
	run: (volumeName: string, input: VolumeMoveInput) => void;
	cancel: () => void;
}

// The volume copy/move transfer (issue 238): the endpoint always answers
// NDJSON, so the dialog drives it through the stream transport. Progress
// events fold into the single-volume progress state; the terminal complete
// event yields the envelope and invalidates both machines' volume queries
// (a partial result is success-shaped — the data crossed); a terminal
// error event surfaces as error.
export function useVolumeMoveStream(
	machineId: string,
): UseVolumeMoveStreamResult {
	const queryClient = useQueryClient();
	const [progress, setProgress] = useState<VolumeMoveProgress | null>(null);
	const [result, setResult] = useState<VolumeMoveResult | null>(null);
	const { isRunning, error, start, cancel } = useNdjsonOperation();

	const run = useCallback(
		(volumeName: string, input: VolumeMoveInput) => {
			setProgress(null);
			setResult(null);
			start({
				url: volumeMoveStreamUrl(machineId, volumeName),
				init: { method: "POST", body: JSON.stringify(input) },
				onEvent: (value) => {
					if (!isVolumeMoveStreamEvent(value)) {
						console.warn(
							"Skipping malformed stream event (unexpected shape):",
							value,
						);
						return;
					}
					if (value.type === "volume") {
						setProgress({
							phase: value.phase ?? "start",
							...(value.bytes !== undefined ? { bytes: value.bytes } : {}),
						});
					} else if (value.type === "complete") {
						if (value.result) {
							setResult(value.result);
							// The both-machines invalidation — one owner for the
							// rule (useVolumes.invalidateVolumeMoveResult).
							invalidateVolumeMoveResult(queryClient, value.result);
						} else {
							console.warn(
								"Skipping complete event without a transfer envelope:",
								value,
							);
						}
					}
				},
			});
		},
		[machineId, queryClient, start],
	);

	return { isRunning, progress, result, error, run, cancel };
}
