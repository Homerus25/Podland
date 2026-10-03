import { invalidateMoveResult } from "@features/quadlets/hooks/useQuadlets";
import type {
	QuadletMoveInput,
	QuadletMoveResult,
} from "@features/quadlets/services/quadletsApi";
import { moveQuadletStreamUrl } from "@features/quadlets/services/quadletsApi";
import { useNdjsonOperation } from "@shared/streaming/streamTransport";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";

// One volume's live data-phase state, folded from the stream's per-volume
// progress events (backend/internal/domain/quadlets/dto.go QuadletMoveEvent):
// the latest phase per volume wins, first-seen order is kept. The backend
// emits a "bytes" event per MiB crossed, so events update in place instead
// of accumulating.
export interface QuadletMoveVolumeProgress {
	ref: string;
	podmanVolume: string;
	phase: "start" | "bytes" | "done" | "skip";
	bytes?: number;
	reason?: string;
}

// Event shape mirror of the backend's QuadletMoveEvent (issue 214):
// per-volume progress events, then exactly one terminal event — the
// transfer envelope on "complete", the mapped code/message on "error".
interface QuadletMoveStreamEvent {
	operationId: string;
	type: "volume" | "complete" | "error";
	machineId: string;
	ref?: string;
	podmanVolume?: string;
	phase?: QuadletMoveVolumeProgress["phase"];
	bytes?: number;
	reason?: string;
	result?: QuadletMoveResult;
	error?: { code: string; message: string };
}

function isQuadletMoveStreamEvent(
	value: unknown,
): value is QuadletMoveStreamEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	return (
		typeof event.operationId === "string" &&
		(event.type === "volume" ||
			event.type === "complete" ||
			event.type === "error")
	);
}

function foldProgress(
	current: QuadletMoveVolumeProgress[],
	event: QuadletMoveStreamEvent,
): QuadletMoveVolumeProgress[] {
	const key = event.podmanVolume ?? event.ref ?? "";
	const entry: QuadletMoveVolumeProgress = {
		ref: event.ref ?? "",
		podmanVolume: key,
		phase: event.phase ?? "start",
		...(event.bytes !== undefined ? { bytes: event.bytes } : {}),
		...(event.reason !== undefined ? { reason: event.reason } : {}),
	};
	const index = current.findIndex((item) => item.podmanVolume === key);
	if (index === -1) return [...current, entry];
	return current.map((item, at) => (at === index ? entry : item));
}

export interface UseQuadletMoveStreamResult {
	isRunning: boolean;
	// Per-volume data-phase progress, one entry per volume, latest phase
	// winning. Empty until the first volume event arrives.
	progress: QuadletMoveVolumeProgress[];
	// The transfer envelope from the terminal complete event — including the
	// volumeMigration block. Null until then and on any failure.
	result: QuadletMoveResult | null;
	// A terminal error event (e.g. VOLUME_MIGRATION_TARGET_NOT_EMPTY) or a
	// transport failure — never an abort.
	error: { code: string; message: string } | null;
	run: (quadletId: string, input: QuadletMoveInput) => void;
	cancel: () => void;
}

// The migrating half of the move/copy transfer (issue 214): the same move
// endpoint, answered as an NDJSON operation because migrateVolumeData
// switches the transport. Per-volume progress events fold into the progress
// list; the terminal complete event yields the envelope and invalidates both
// machines' quadlet queries exactly like the plain mutation's onSuccess (a
// partial result is success-shaped — the target install stands); a terminal
// error event surfaces as error. The file-only path keeps riding the plain
// mutation — this hook exists only for the checked checkbox.
export function useQuadletMoveStream(
	machineId: string,
): UseQuadletMoveStreamResult {
	const queryClient = useQueryClient();
	const [progress, setProgress] = useState<QuadletMoveVolumeProgress[]>([]);
	const [result, setResult] = useState<QuadletMoveResult | null>(null);
	const { isRunning, error, start, cancel } = useNdjsonOperation();

	const run = useCallback(
		(quadletId: string, input: QuadletMoveInput) => {
			setProgress([]);
			setResult(null);
			start({
				url: moveQuadletStreamUrl(machineId, quadletId),
				init: { method: "POST", body: JSON.stringify(input) },
				onEvent: (value) => {
					if (!isQuadletMoveStreamEvent(value)) {
						console.warn(
							"Skipping malformed stream event (unexpected shape):",
							value,
						);
						return;
					}
					if (value.type === "volume") {
						setProgress((current) => foldProgress(current, value));
					} else if (value.type === "complete") {
						if (value.result) {
							setResult(value.result);
							// The same both-machines invalidation the plain
							// mutation's onSuccess performs — one owner for
							// the rule (useQuadlets.invalidateMoveResult).
							invalidateMoveResult(queryClient, value.result);
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
