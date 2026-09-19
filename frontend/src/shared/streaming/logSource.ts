import {
	type BufferedLogs,
	followLogStream,
	type LogEvent,
} from "@shared/streaming/logStream";
import { api, getApiError } from "@shared/utils/api";

// How a log viewer obtains its lines. The same contract serves any backend
// log endpoint (container and quadlet logs today): the caller wires the two
// calls to its own URLs and params.
export interface LogSource {
	// Fetches the last `tail` lines without following.
	fetchTail: (tail: number) => Promise<string[]>;
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
		follow: (tail, onEvent, signal) => {
			const query = new URLSearchParams({
				tail: String(tail),
				follow: "true",
			});
			return followLogStream(`${url}?${query}`, onEvent, signal);
		},
	};
}
