// The transfer-hooks kit (issue 273, 2026-10-08 architecture-review
// candidate 5): the one module behind the three cross-machine move/copy
// surfaces (quadlet, volume, secret). It owns the ritual every transfer
// shares — the target-scoped preview query, the both-machines list
// invalidation, and the move-stream lifecycle over the stream transport —
// and a surface adapts with its key constructor and fetcher, its URL
// builder, event vocabulary, shape guard, and progress fold. The plain move
// mutations stay feature-local: with the invalidation rule owned here, each
// is two lines.
import type { MachineResource } from "@shared/queries/queryKeys";
import { invalidateMachineResources } from "@shared/queries/useListInvalidatingMutation";
import { useNdjsonOperation } from "@shared/streaming/streamTransport";
import type { ApiError } from "@shared/types/api";
import {
	type QueryClient,
	useQuery,
	useQueryClient,
} from "@tanstack/react-query";
import { useCallback, useState } from "react";

// The structural machine-id slice every transfer envelope carries
// (QuadletMoveResult, VolumeMoveResult, and SecretMoveResult all answer
// with it): a transfer touches two machines, so both lists invalidate from
// what the server answered rather than from closure assumptions. A
// "partial" result is still a success-shaped answer — the target install
// stands — so both lists refresh there too.
interface MoveResultMachines {
	sourceMachineId: string;
	targetMachineId: string;
}

// The both-machines list invalidation, owned once: the plain mutations'
// onSuccess and the move-stream factory's complete handler both call this.
export function invalidateMoveResult(
	queryClient: QueryClient,
	result: MoveResultMachines,
	resource: MachineResource,
): void {
	invalidateMachineResources(queryClient, result.sourceMachineId, resource);
	invalidateMachineResources(queryClient, result.targetMachineId, resource);
}

// The target-scoped preview query behind every move/copy dialog: keyed by
// the chosen target so a target change refetches, with the empty-string
// placeholder while no target exists (the disabled query still needs a
// stable key), disabled until a target exists — the dialog defaults the
// select to the first writable machine once the machines list arrives — and
// guarded so a null target can never fetch. A surface adapts with its key
// constructor (the shared query-key module stays the single owner of the
// key vocabulary) and fetcher.
export function useMovePreview<TPreview>(
	queryKey: (targetMachineId: string) => readonly unknown[],
	fetcher: (targetMachineId: string) => Promise<TPreview>,
	targetMachineId: string | null,
) {
	return useQuery({
		queryKey: queryKey(targetMachineId ?? ""),
		queryFn: () => {
			if (targetMachineId === null) {
				throw new Error("move preview requires a target machine");
			}
			return fetcher(targetMachineId);
		},
		enabled: targetMachineId !== null,
	});
}

// The structural base every move stream's events share: the operation
// envelope (operationId + type) the stream transport terminates on. The
// factory branches on this base alone and imports no domain event types; a
// surface's event vocabulary narrows it through its own shape guard.
interface MoveStreamEventBase {
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
// lists on both machines a completed transfer invalidates, the URL builder
// (item id to the full stream URL), the shape guard over the surface's
// event vocabulary, the progress fold (the event folded into the current
// state; a single-value fold declares only the event), and the progress
// state a run resets to.
export interface MoveStreamConfig<
	TEvent extends MoveStreamEventBase,
	TProgress,
> {
	resource: MachineResource;
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

// The move-stream lifecycle over the NDJSON operation hook: the run/cancel
// lifecycle, reset-on-run, the malformed-event warn-and-skip, the
// complete-without-envelope warn-and-skip, the terminal handling (the
// complete event's envelope lands in the result and invalidates both
// machines' lists), and the invalidation call. Error terminals stay the
// stream transport's, as today. A surface keeps its event vocabulary, shape
// guard, progress fold, and URL builder.
export function useMoveStream<
	TEvent extends MoveStreamEventBase,
	TProgress,
	TResult extends MoveResultMachines,
>(
	config: MoveStreamConfig<TEvent, TProgress>,
): UseMoveStreamResult<TProgress, TResult> {
	const { resource, url, isEvent, foldProgress, initialProgress } = config;
	const queryClient = useQueryClient();
	const [progress, setProgress] = useState<TProgress>(initialProgress);
	const [result, setResult] = useState<TResult | null>(null);
	const { isRunning, error, start, cancel } = useNdjsonOperation();

	const run = useCallback(
		<TInput>(id: string, input: TInput) => {
			setProgress(initialProgress);
			setResult(null);
			start({
				url: url(id),
				init: { method: "POST", body: JSON.stringify(input) },
				onEvent: (value) => {
					if (!isEvent(value)) {
						console.warn(
							"Skipping malformed stream event (unexpected shape):",
							value,
						);
						return;
					}
					if (isCompleteEvent<TResult>(value)) {
						if (value.result) {
							setResult(value.result);
							invalidateMoveResult(queryClient, value.result, resource);
						} else {
							console.warn(
								"Skipping complete event without a transfer envelope:",
								value,
							);
						}
						return;
					}
					// Error terminals stay the stream transport's: it surfaces them
					// as the error state, so there is nothing to fold here.
					if (value.type === "error") return;
					setProgress((current) => foldProgress(value, current));
				},
			});
		},
		[queryClient, start, resource, url, isEvent, foldProgress, initialProgress],
	);

	return { isRunning, progress, result, error, run, cancel };
}
