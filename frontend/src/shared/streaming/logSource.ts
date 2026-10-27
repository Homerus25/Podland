import {
	type BufferedLogs,
	followLogStream,
	type LogEvent,
} from "@shared/streaming/logStream";
import type { ApiError } from "@shared/types/api";
import { api, genericError, getApiError } from "@shared/utils/api";

// Narrows a LogSource rejection to an ApiError. Sources reject with
// ApiError-shaped objects (the buffered fetches wrap through getApiError,
// the stream helper builds envelopes), so this passes a well-formed code
// and message through; anything else collapses to the generic one.
export function toApiError(error: unknown): ApiError {
	if (typeof error === "object" && error !== null) {
		const candidate = error as { code?: unknown; message?: unknown };
		if (
			typeof candidate.code === "string" &&
			typeof candidate.message === "string"
		) {
			return { code: candidate.code, message: candidate.message };
		}
	}
	return { ...genericError };
}

// How a log viewer obtains its lines. The same contract serves any backend
// log endpoint (container and quadlet logs today): the caller wires the two
// calls to its own URLs and params.
export interface LogSource {
	// Fetches the last `tail` lines without following.
	fetchTail: (tail: number) => Promise<string[]>;
	// Fetches the target's entire log history as raw text (the buffered
	// endpoint without a tail param — the backend default is all lines).
	// A one-shot snapshot for downloads; it never touches the viewer's
	// tail view or an active follow stream.
	fetchAll: () => Promise<string>;
	// Opens the live stream; calls onEvent per streamed log event and resolves
	// when the stream ends. Must honor the AbortSignal so toggling follow off,
	// closing the viewer, or unmounting tears down the connection.
	follow: (
		tail: number,
		onEvent: (event: LogEvent) => void,
		signal: AbortSignal,
	) => Promise<void>;
}

// Builds a LogSource for one log endpoint from its pre-built machine-scoped
// path/url pair: the caller keeps the machine vocabulary (machineApiPath for
// the axios call, machineApiUrl for the stream), the factory owns the whole
// log-endpoint ritual — the buffered tail fetch with its split-and-filter
// parse, the URLSearchParams follow query, and the ApiError error wrap.
export function createLogSource(path: string, url: string): LogSource {
	return {
		fetchTail: async (tail) => {
			try {
				const response = await api.get<BufferedLogs>(path, {
					params: { tail },
				});
				return response.data.logs.split("\n").filter((line) => line !== "");
			} catch (error) {
				throw getApiError(error);
			}
		},
		fetchAll: async () => {
			try {
				const response = await api.get<BufferedLogs>(path);
				return response.data.logs;
			} catch (error) {
				throw getApiError(error);
			}
		},
		follow: (tail, onEvent, signal) => {
			const query = new URLSearchParams({
				tail: String(tail),
				follow: "true",
			});
			return followLogStream(`${url}?${query}`, onEvent, signal);
		},
	};
}
