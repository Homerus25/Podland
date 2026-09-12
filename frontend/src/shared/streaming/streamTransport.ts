import { consumeNdjsonValues } from "@shared/streaming/ndjsonStream";
import type { ApiError } from "@shared/types/api";
import {
	genericError,
	getApiErrorFromResponse,
	getBearerToken,
} from "@shared/utils/api";
import { useCallback, useEffect, useRef, useState } from "react";

export interface StreamNdjsonInit {
	method?: string;
	body?: string;
	signal: AbortSignal;
}

// The streaming fetch every NDJSON endpoint uses: it injects the Bearer
// credential from the shared token holder, sends Content-Type only when a
// body is present, turns non-ok responses into the displayable error
// envelope and every other failure into the generic sentinel, leaves abort
// errors raw for the lifecycle owner to interpret, and pumps every parsed
// NDJSON value to the caller without judging its shape. Resolves on clean
// stream end.
export async function streamNdjson(
	url: string,
	init: StreamNdjsonInit,
	onValue: (value: unknown) => void,
): Promise<void> {
	const token = getBearerToken();
	let response: Response;
	try {
		response = await fetch(url, {
			method: init.method,
			headers: {
				...(init.body !== undefined
					? { "Content-Type": "application/json" }
					: {}),
				...(token ? { Authorization: `Bearer ${token}` } : {}),
			},
			body: init.body,
			signal: init.signal,
		});
	} catch (fetchError) {
		if (init.signal.aborted) throw fetchError;
		throw { ...genericError };
	}

	if (!response.ok) {
		throw await getApiErrorFromResponse(response);
	}

	if (!response.body) {
		throw { ...genericError };
	}

	try {
		await consumeNdjsonValues(response.body, onValue);
	} catch (streamError) {
		if (init.signal.aborted) throw streamError;
		throw { ...genericError };
	}
}

export interface NdjsonOperationOptions {
	url: string;
	init?: { method?: string; body?: string };
	onEvent: (event: unknown) => void;
}

export interface NdjsonOperation {
	isRunning: boolean;
	// Error events and transport failures, never aborts.
	error: ApiError | null;
	start: (options: NdjsonOperationOptions) => void;
	cancel: () => void;
}

// The backend's operation streams terminate with a complete/error event
// carrying a string operationId; every other type is a non-terminal
// vocabulary owned by the feature (progress, log, ...).
function terminalType(value: unknown): "complete" | "error" | null {
	if (typeof value !== "object" || value === null) return null;
	const event = value as Record<string, unknown>;
	if (typeof event.operationId !== "string") return null;
	if (event.type === "complete" || event.type === "error") return event.type;
	return null;
}

// The operation lifecycle both mutation-stream flows share: starting a new
// operation supersedes the previous one (its late events and terminal side
// effects reach nobody), terminal complete/error events end the run exactly
// once, an error event or transport failure surfaces as error, an abort
// never does, and unmounting tears the connection down.
export function useNdjsonOperation(): NdjsonOperation {
	const [isRunning, setIsRunning] = useState(false);
	const [error, setError] = useState<ApiError | null>(null);
	const abortRef = useRef<AbortController | null>(null);

	const cancel = useCallback(() => {
		abortRef.current?.abort();
	}, []);

	useEffect(
		() => () => {
			abortRef.current?.abort();
		},
		[],
	);

	const start = useCallback((options: NdjsonOperationOptions) => {
		abortRef.current?.abort();
		const controller = new AbortController();
		abortRef.current = controller;

		setError(null);
		setIsRunning(true);

		// Terminal cleanup. Returns false once this run has already finished or
		// been superseded, so a second terminal event cannot rerun its side
		// effects.
		const finish = (): boolean => {
			if (abortRef.current !== controller) return false;
			abortRef.current = null;
			setIsRunning(false);
			return true;
		};

		void (async () => {
			try {
				await streamNdjson(
					options.url,
					{ ...options.init, signal: controller.signal },
					(value) => {
						if (abortRef.current !== controller || controller.signal.aborted)
							return;
						options.onEvent(value);
						const terminal = terminalType(value);
						if (!terminal) return;
						if (finish() && terminal === "error") {
							const event = value as {
								error?: { code?: string; message?: string };
							};
							setError({
								code: event.error?.code ?? genericError.code,
								message: event.error?.message ?? genericError.message,
							});
						}
					},
				);
				finish();
			} catch (streamError) {
				// streamNdjson rejects raw abort errors and ApiError-shaped values
				// only; aborts are cancellation, not failure.
				if (finish() && !controller.signal.aborted)
					setError(streamError as ApiError);
			}
		})();
	}, []);

	return { isRunning, error, start, cancel };
}
