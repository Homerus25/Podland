import { streamNdjson } from "@shared/streaming/streamTransport";
import type { ApiError } from "@shared/types/api";

// A single log line as streamed by the backend's log endpoints in follow mode
// (e.g. backend/internal/domain/containers/dto.go ContainerLogEvent).
export interface LogEvent {
	timestamp: string;
	level: string;
	message: string;
}

export function isLogEvent(value: unknown): value is LogEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	return (
		typeof event.timestamp === "string" &&
		typeof event.level === "string" &&
		typeof event.message === "string"
	);
}

// The backend encodes stream failures as ERROR-level events whose message is
// "CODE: human message" — the shared log-stream terminal contract of the
// containers and quadlets follow-mode handlers (both driven by
// backend/internal/streaming). Only UPPER_SNAKE codes with at least one
// underscore qualify: stderr lines also arrive as level ERROR with arbitrary
// text ("FATAL: could not bind"), and the backend's error codes are always
// multi-word.
export function parseLogErrorEvent(event: LogEvent): ApiError | null {
	if (event.level !== "ERROR") return null;
	const separator = event.message.indexOf(": ");
	if (separator === -1) return null;
	const code = event.message.slice(0, separator);
	if (!/^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+$/.test(code)) return null;
	return { code, message: event.message.slice(separator + 2) };
}

// Streams a URL as an NDJSON log event stream on the shared transport
// function (auth, error envelopes, abort handling are streamNdjson's).
// Emits LogEvent-shaped values; other values are warned and skipped. Resolves
// when the stream ends, rejects with an ApiError on transport failure or a
// non-ok response, and tears down the connection on abort via the signal.
export async function followLogStream(
	url: string,
	onEvent: (event: LogEvent) => void,
	signal: AbortSignal,
): Promise<void> {
	await streamNdjson(url, { signal }, (value) => {
		if (isLogEvent(value)) {
			onEvent(value);
		} else {
			console.warn("Skipping non-log stream value (unexpected shape):", value);
		}
	});
}
