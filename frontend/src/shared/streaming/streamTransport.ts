import { consumeNdjsonValues } from "@shared/streaming/ndjsonStream";
import type { ApiError } from "@shared/types/api";
import {
	fireSessionExpired,
	genericError,
	getApiErrorFromResponse,
	getBearerToken,
	handleUnauthorized,
} from "@shared/utils/api";
import { useCallback, useEffect, useRef, useState } from "react";

export interface StreamNdjsonInit {
	method?: string;
	body?: string;
	signal: AbortSignal;
}

// One wire attempt: the Bearer preamble from the shared token holder,
// Content-Type only when a body is present, and abort errors rethrown raw
// for the lifecycle owner to interpret.
async function openNdjsonStream(
	url: string,
	init: StreamNdjsonInit,
): Promise<Response> {
	const token = getBearerToken();
	try {
		return await fetch(url, {
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
}

// Races the signal against `promise` without cancelling it: the refresh is
// single-flight and may be shared with other 401s, so an abort stops this
// waiter only.
function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
	if (signal.aborted) return Promise.reject(signal.reason);
	return new Promise<T>((resolve, reject) => {
		const onAbort = () => reject(signal.reason);
		signal.addEventListener("abort", onAbort, { once: true });
		promise.then(
			(value) => {
				signal.removeEventListener("abort", onAbort);
				resolve(value);
			},
			(error: unknown) => {
				signal.removeEventListener("abort", onAbort);
				reject(error);
			},
		);
	});
}

// The streaming fetch every NDJSON endpoint uses: it injects the Bearer
// credential from the shared token holder, sends Content-Type only when a
// body is present, turns non-ok responses into the displayable error
// envelope and every other failure into the generic sentinel, leaves abort
// errors raw for the lifecycle owner to interpret, and pumps every parsed
// NDJSON value to the caller without judging its shape. A 401 runs the
// shared 401 policy: one refresh, then one re-fetch; a rejected refresh or
// a 401 on the re-fetch (the marked replay) surfaces the envelope with the
// session-expired seam already fired. Resolves on clean stream end.
export async function streamNdjson(
	url: string,
	init: StreamNdjsonInit,
	onValue: (value: unknown) => void,
): Promise<void> {
	let response = await openNdjsonStream(url, init);

	if (response.status === 401) {
		// A 401 means the request was rejected before execution, so the
		// re-fetch (including a streamed POST) is safe.
		if (!(await abortable(handleUnauthorized(), init.signal))) {
			throw await getApiErrorFromResponse(response);
		}
		response = await openNdjsonStream(url, init);
		if (response.status === 401) {
			fireSessionExpired();
			throw await getApiErrorFromResponse(response);
		}
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

// The plain runner behind every operation stream's event loop (spec 486
// review): it owns the loop's ritual — every parsed value is dispatched to
// `onEvent` until the FIRST terminal event, which is dispatched to both
// handlers and ends dispatch, so a second terminal or any late event reaches
// nobody (exactly-one-terminal). The hook below delegates to it, and the
// transfer kit's plain move runner does too, so a batch transfer and the
// operation flows cannot drift on terminal semantics. It resolves on clean
// stream end — with or without a terminal, the outcome mapping is the
// caller's — and rejects per `streamNdjson`: raw abort errors and
// ApiError-shaped values only.
export interface NdjsonOperationHandlers {
	onEvent: (value: unknown) => void;
	onTerminal: (type: "complete" | "error", value: unknown) => void;
}

export async function runNdjsonOperation(
	url: string,
	init: { method?: string; body?: string } | undefined,
	signal: AbortSignal,
	handlers: NdjsonOperationHandlers,
): Promise<void> {
	let finished = false;
	await streamNdjson(url, { ...init, signal }, (value) => {
		if (finished) return;
		handlers.onEvent(value);
		const terminal = terminalType(value);
		if (!terminal) return;
		finished = true;
		handlers.onTerminal(terminal, value);
	});
}

// The operation lifecycle both mutation-stream flows share: starting a new
// operation supersedes the previous one (its late events and terminal side
// effects reach nobody), terminal complete/error events end the run exactly
// once, an error event or transport failure surfaces as error, an abort
// never does, and unmounting tears the connection down. The event loop and
// the terminal semantics are the plain runner's above; this hook owns the
// state around them — the abort controller, the supersede identity, and
// the running/error state.
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
				await runNdjsonOperation(options.url, options.init, controller.signal, {
					onEvent: (value) => {
						if (abortRef.current !== controller || controller.signal.aborted)
							return;
						options.onEvent(value);
					},
					onTerminal: (terminal, value) => {
						if (controller.signal.aborted || !finish()) return;
						if (terminal === "error") {
							const event = value as {
								error?: { code?: string; message?: string };
							};
							setError({
								code: event.error?.code ?? genericError.code,
								message: event.error?.message ?? genericError.message,
							});
						}
					},
				});
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
