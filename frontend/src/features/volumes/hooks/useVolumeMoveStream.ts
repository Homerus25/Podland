import type {
	VolumeMoveInput,
	VolumeMoveResult,
} from "@features/volumes/services/volumesApi";
import { volumeMoveStreamUrl } from "@features/volumes/services/volumesApi";
import {
	type MoveStreamConfig,
	useMoveStream,
} from "@shared/transfers/transferHooks";
import { useMemo } from "react";

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
	hostId: string;
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

// The single-value fold: the latest phase wins, and the bytes ride along
// when the event carries them.
function foldProgress(event: VolumeMoveStreamEvent): VolumeMoveProgress | null {
	return {
		phase: event.phase ?? "start",
		...(event.bytes !== undefined ? { bytes: event.bytes } : {}),
	};
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
// NDJSON, so the dialog drives it through the stream transport. The
// run/cancel lifecycle, reset-on-run, the malformed-event and
// complete-without-envelope skips, the terminal handling, and the
// both-hosts invalidation are the shared transfer-hooks module's (issue
// 273); this adapter keeps the volume domain — the event vocabulary, shape
// guard, and single-value progress fold — and supplies the URL builder.
export function useVolumeMoveStream(hostId: string): UseVolumeMoveStreamResult {
	const config = useMemo<
		MoveStreamConfig<VolumeMoveStreamEvent, VolumeMoveProgress | null>
	>(
		() => ({
			resource: "volumes",
			url: (volumeName) => volumeMoveStreamUrl(hostId, volumeName),
			isEvent: isVolumeMoveStreamEvent,
			foldProgress,
			initialProgress: null,
		}),
		[hostId],
	);
	return useMoveStream<
		VolumeMoveStreamEvent,
		VolumeMoveProgress | null,
		VolumeMoveResult
	>(config);
}
