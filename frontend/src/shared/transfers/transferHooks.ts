// The transfer-hooks kit (issue 273, 2026-10-08 architecture-review
// candidate 5): the one module behind the three cross-host move/copy
// surfaces (quadlet, volume, secret). It owns the ritual every transfer
// shares — the target-scoped preview query, the both-hosts list
// invalidation, and the move-stream lifecycle over the stream transport —
// and a surface adapts with its key constructor and fetcher, its URL
// builder, event vocabulary, shape guard, and progress fold. The plain move
// mutations stay feature-local: with the invalidation rule owned here, each
// is two lines.
import type { HostResource } from "@shared/queries/queryKeys";
import { invalidateHostResources } from "@shared/queries/useListInvalidatingMutation";
import { runNdjsonOperation } from "@shared/streaming/streamTransport";
import type { ApiError } from "@shared/types/api";
import { genericError, getApiError } from "@shared/utils/api";
import {
	type QueryClient,
	type UseQueryResult,
	useQueries,
	useQuery,
	useQueryClient,
} from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

// The structural host-id slice every transfer envelope carries
// (QuadletMoveResult, VolumeMoveResult, and SecretMoveResult all answer
// with it): a transfer touches two hosts, so both lists invalidate from
// what the server answered rather than from closure assumptions. A
// "partial" result is still a success-shaped answer — the target install
// stands — so both lists refresh there too.
export interface MoveResultHosts {
	sourceHostId: string;
	targetHostId: string;
}

// The both-hosts list invalidation, owned once: the plain mutations'
// onSuccess and the move-stream factory's complete handler both call this.
export function invalidateMoveResult(
	queryClient: QueryClient,
	result: MoveResultHosts,
	resource: HostResource,
): void {
	invalidateHostResources(queryClient, result.sourceHostId, resource);
	invalidateHostResources(queryClient, result.targetHostId, resource);
}

// The target-scoped preview query behind every move/copy dialog: keyed by
// the chosen target so a target change refetches, with the empty-string
// placeholder while no target exists (the disabled query still needs a
// stable key), disabled until a target exists — the dialog defaults the
// select to the first writable host once the hosts list arrives — and
// guarded so a null target can never fetch. A surface adapts with its key
// constructor (the shared query-key module stays the single owner of the
// key vocabulary) and fetcher.
export function movePreviewQueryOptions<TPreview>(
	queryKey: (targetHostId: string) => readonly unknown[],
	fetcher: (targetHostId: string) => Promise<TPreview>,
	targetHostId: string | null,
) {
	return {
		queryKey: queryKey(targetHostId ?? ""),
		queryFn: () => {
			if (targetHostId === null) {
				throw new Error("move preview requires a target host");
			}
			return fetcher(targetHostId);
		},
		enabled: targetHostId !== null,
	};
}

export function useMovePreview<TPreview>(
	queryKey: (targetHostId: string) => readonly unknown[],
	fetcher: (targetHostId: string) => Promise<TPreview>,
	targetHostId: string | null,
) {
	return useQuery(movePreviewQueryOptions(queryKey, fetcher, targetHostId));
}

// The batch twin of useMovePreview (spec 486, ticket #487): a hook cannot be
// called in a loop, so a batch transfer previews N items through useQueries
// over the same per-item options. The key constructor and fetcher take the
// item id as well as the target — the per-item key is
// (host, item, target) — and the results come back in the order the ids were
// given, which is the order the dialog renders and runs them in. The ritual
// itself (the null-target guard, the empty-string placeholder key, the
// enabled gate) stays movePreviewQueryOptions', so a batch and a single
// transfer cannot ask the backend different questions.
export function useMovePreviews<TPreview>(
	queryKey: (id: string, targetHostId: string) => readonly unknown[],
	fetcher: (id: string, targetHostId: string) => Promise<TPreview>,
	ids: string[],
	targetHostId: string | null,
): UseQueryResult<TPreview>[] {
	return useQueries({
		queries: ids.map((id) =>
			movePreviewQueryOptions(
				(target) => queryKey(id, target),
				(target) => fetcher(id, target),
				targetHostId,
			),
		),
	});
}

// The failure detail for one item's run, owned once (spec 486 review): a
// stream refusal is an ApiError-shaped plain value — the stream transport's
// contract, and the same value the single-item hook stores as its `error` —
// so its own message is the detail a batch report exists to show.
// `getApiError` only passes axios envelopes through and would collapse the
// refusal to the generic message. An axios error is an Error instance, so it
// is never mistaken for the plain shape and keeps its envelope's message.
export function transferErrorMessage(error: unknown): string {
	if (
		typeof error === "object" &&
		error !== null &&
		!(error instanceof Error) &&
		"code" in error &&
		typeof (error as { code: unknown }).code === "string" &&
		"message" in error &&
		typeof (error as { message: unknown }).message === "string"
	) {
		return (error as { message: string }).message;
	}
	return getApiError(error).message;
}

// The structural base every move stream's events share: the operation
// envelope (operationId + type) the stream transport terminates on. The
// factory branches on this base alone and imports no domain event types; a
// surface's event vocabulary narrows it through its own shape guard.
export interface MoveStreamEventBase {
	operationId: string;
	type: string;
}

// A terminal complete event carrying the transfer envelope — the structural
// slice the factory's completion handling reads.
interface MoveStreamCompleteEvent<Result> extends MoveStreamEventBase {
	type: "complete";
	result?: Result;
}

function isCompleteEvent<Result>(
	event: MoveStreamEventBase,
): event is MoveStreamCompleteEvent<Result> {
	return event.type === "complete";
}

// A surface's adaptation of the move-stream factory: the resource whose
// lists on both hosts a completed transfer invalidates, the URL builder
// (item id to the full stream URL), the shape guard over the surface's
// event vocabulary, the progress fold (the event folded into the current
// state; a single-value fold declares only the event), and the progress
// state a run resets to.
export interface MoveStreamConfig<
	TEvent extends MoveStreamEventBase,
	TProgress,
> {
	resource: HostResource;
	url: (id: string) => string;
	isEvent: (value: unknown) => value is TEvent;
	foldProgress: (event: TEvent, current: TProgress) => TProgress;
	initialProgress: TProgress;
}

interface UseMoveStreamResult<TProgress, TResult> {
	isRunning: boolean;
	progress: TProgress;
	// The transfer envelope from the terminal complete event. Null until
	// then and on any failure.
	result: TResult | null;
	// A terminal error event or a transport failure — never an abort.
	error: ApiError | null;
	run: <TInput>(id: string, input: TInput) => void;
	cancel: () => void;
}

// What one run of the move stream is handed: the signal the caller aborts to
// cancel, and the progress callback the fold reports through. A hook cannot
// be called in a loop, so a batch transfer drives the same event loop through
// this plain function instead of the hook.
export interface MoveStreamRunOptions<TProgress> {
	signal: AbortSignal;
	onProgress: (progress: TProgress) => void;
}

// The move-stream event loop, as a plain function (spec 486, ticket #487):
// the loop itself and the exactly-one-terminal ritual are the stream
// transport's plain runner's — the same one the operation hook delegates
// to, so a batch transfer and every other operation stream cannot drift on
// terminal semantics (spec 486 review). The domain stays here: skip
// malformed events with a warn, fold progress and hand it to the caller,
// capture the terminal complete event's envelope and invalidate both hosts'
// lists, reject with a terminal error event's code and message, and reject
// with the generic error when the stream ends without an envelope. The hook
// delegates here so the loop exists once; the batch transfers call it
// directly, sequentially, one item at a time.
export async function runMoveStream<
	TEvent extends MoveStreamEventBase,
	TProgress,
	TResult extends MoveResultHosts,
>(
	config: MoveStreamConfig<TEvent, TProgress>,
	queryClient: QueryClient,
	id: string,
	input: unknown,
	options: MoveStreamRunOptions<TProgress>,
): Promise<TResult> {
	const { resource, url, isEvent, foldProgress, initialProgress } = config;
	// The loop's captures live in one holder: the callback runs inside the
	// transport's runner, so the compiler cannot see its assignments.
	const captured: { envelope: TResult | null; error: ApiError | null } = {
		envelope: null,
		error: null,
	};
	let progress = initialProgress;
	await runNdjsonOperation(
		url(id),
		{ method: "POST", body: JSON.stringify(input) },
		options.signal,
		{
			onEvent: (value) => {
				if (!isEvent(value)) {
					console.warn(
						"Skipping malformed stream event (unexpected shape):",
						value,
					);
					return;
				}
				// Terminals are the runner's business (below); progress events
				// fold.
				if (value.type === "complete" || value.type === "error") return;
				progress = foldProgress(value, progress);
				options.onProgress(progress);
			},
			onTerminal: (terminal, value) => {
				if (!isEvent(value)) {
					console.warn(
						"Skipping malformed stream event (unexpected shape):",
						value,
					);
					return;
				}
				if (terminal === "complete") {
					if (isCompleteEvent<TResult>(value)) {
						if (value.result) {
							captured.envelope = value.result;
							invalidateMoveResult(queryClient, value.result, resource);
						} else {
							console.warn(
								"Skipping complete event without a transfer envelope:",
								value,
							);
						}
					}
					return;
				}
				// Error terminals end the run as a rejection; they are never
				// folded into progress.
				const event = value as unknown as {
					error?: { code?: string; message?: string };
				};
				captured.error = {
					code: event.error?.code ?? genericError.code,
					message: event.error?.message ?? genericError.message,
				};
			},
		},
	);
	const envelope: TResult | null = captured.envelope;
	const terminalError: ApiError | null = captured.error;
	if (terminalError) throw terminalError;
	if (!envelope) throw { ...genericError };
	return envelope;
}

// The move-stream lifecycle over the plain runner: the run/cancel lifecycle,
// reset-on-run, the supersede rule (a new run aborts the old one, whose late
// events and settle reach nobody), the abort-on-unmount teardown, and the
// abort-is-not-an-error rule. The event loop itself is runMoveStream's, so a
// batch transfer and this hook cannot drift. The state wiring is this hook's
// own — the transport's operation hook carries no progress/result state —
// but the loop and the terminal semantics underneath both are the
// transport's plain runner's, shared with useNdjsonOperation (spec 486
// review). A surface keeps its event vocabulary, shape guard, progress
// fold, and URL builder.
export function useMoveStream<
	TEvent extends MoveStreamEventBase,
	TProgress,
	TResult extends MoveResultHosts,
>(
	config: MoveStreamConfig<TEvent, TProgress>,
): UseMoveStreamResult<TProgress, TResult> {
	const { resource, url, isEvent, foldProgress, initialProgress } = config;
	const queryClient = useQueryClient();
	const [progress, setProgress] = useState<TProgress>(initialProgress);
	const [result, setResult] = useState<TResult | null>(null);
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

	const run = useCallback(
		<TInput>(id: string, input: TInput) => {
			abortRef.current?.abort();
			const controller = new AbortController();
			abortRef.current = controller;
			setProgress(initialProgress);
			setResult(null);
			setError(null);
			setIsRunning(true);
			void (async () => {
				try {
					const envelope = await runMoveStream<TEvent, TProgress, TResult>(
						{ resource, url, isEvent, foldProgress, initialProgress },
						queryClient,
						id,
						input,
						{
							signal: controller.signal,
							onProgress: (next) => {
								if (abortRef.current === controller) setProgress(next);
							},
						},
					);
					if (abortRef.current !== controller) return;
					abortRef.current = null;
					setResult(envelope);
					setIsRunning(false);
				} catch (streamError) {
					// streamNdjson rejects raw abort errors and ApiError-shaped
					// values only; aborts are cancellation, not failure.
					if (abortRef.current !== controller) return;
					abortRef.current = null;
					setIsRunning(false);
					if (!controller.signal.aborted) setError(streamError as ApiError);
				}
			})();
		},
		[queryClient, resource, url, isEvent, foldProgress, initialProgress],
	);

	return { isRunning, progress, result, error, run, cancel };
}
