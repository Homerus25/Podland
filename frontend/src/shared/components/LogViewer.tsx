import { Box, Select, Switch, Text, VStack } from "@chakra-ui/react";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { type LogEvent, parseLogErrorEvent } from "@shared/streaming/logStream";
import type { ApiError } from "@shared/types/api";
import { genericError } from "@shared/utils/api";
import { useEffect, useRef, useState } from "react";

// How a log viewer obtains its lines. The same component serves any backend
// log endpoint (container and quadlet logs today): the caller wires
// the two calls to its own URLs and params.
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

const TAIL_OPTIONS = [50, 100, 200, 500];
const DEFAULT_TAIL = 100;

function formatEvent(event: LogEvent): string {
	return `${event.timestamp} ${event.level} ${event.message}`;
}

// Sources reject with ApiError-shaped objects (the stream helper does, and
// sources convert axios errors); anything else collapses to the generic one.
function toApiError(error: unknown): ApiError {
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

// Generic streaming log viewer: initial tail fetch, follow toggle driving a
// live NDJSON stream, auto-scroll while following, and error surfacing. Owns
// one AbortController per follow session; aborting on unmount guarantees no
// background connections leak.
export function LogViewer({
	source,
	maxHeight = "400px",
}: {
	source: LogSource;
	maxHeight?: string;
}) {
	const [tail, setTail] = useState(DEFAULT_TAIL);
	const [lines, setLines] = useState<string[]>([]);
	const [isFollowing, setIsFollowing] = useState(false);
	const [error, setError] = useState<ApiError | null>(null);
	const abortRef = useRef<AbortController | null>(null);
	const scrollRef = useRef<HTMLDivElement>(null);
	const isFollowingRef = useRef(false);
	const mountedRef = useRef(false);

	const loadTail = async (requestedTail: number) => {
		setError(null);
		try {
			setLines(await source.fetchTail(requestedTail));
		} catch (tailError) {
			setLines([]);
			setError(toApiError(tailError));
		}
	};

	const stopFollowing = () => {
		abortRef.current?.abort();
		abortRef.current = null;
		isFollowingRef.current = false;
		setIsFollowing(false);
	};

	const startFollowing = async (requestedTail: number) => {
		abortRef.current?.abort();
		const controller = new AbortController();
		abortRef.current = controller;
		setError(null);
		setIsFollowing(true);
		isFollowingRef.current = true;
		// The backend's follow mode replays the tail before streaming new lines,
		// so the initial fetch's lines are dropped to avoid showing them twice.
		setLines([]);

		try {
			await source.follow(
				requestedTail,
				(event) => {
					if (controller.signal.aborted) return;
					const logError = parseLogErrorEvent(event);
					if (logError) {
						setError(logError);
						return;
					}
					setLines((previous) => [...previous, formatEvent(event)]);
				},
				controller.signal,
			);
		} catch (streamError) {
			// An abort surfaces here as a rejected read; only real failures show,
			// preserving the envelope when the source rejected with one.
			if (!controller.signal.aborted) setError(toApiError(streamError));
		} finally {
			// The stream ended on its own; follow stays on until the user
			// toggles it off (the next toggle aborts a dead controller, a no-op).
			if (abortRef.current === controller) {
				abortRef.current = null;
			}
		}
	};

	const onFollowToggle = () => {
		if (isFollowing) {
			stopFollowing();
		} else {
			void startFollowing(tail);
		}
	};

	// Initial load: fetch the tail once on mount.
	// biome-ignore lint/correctness/useExhaustiveDependencies: mount-only fetch; tail changes are handled by the effect below
	useEffect(() => {
		void loadTail(DEFAULT_TAIL);
		mountedRef.current = true;
	}, []);

	// Tail changes reroute whichever mode is active: a live stream restarts
	// with the new tail, a static view refetches. startFollowing/loadTail are
	// re-created every render, so they must stay out of the dependency array:
	// including them made this effect re-run on every render in an infinite
	// async loop that starved the event loop and hung the test worker.
	// biome-ignore lint/correctness/useExhaustiveDependencies: only tail drives this effect; callbacks are unstable identities
	useEffect(() => {
		if (!mountedRef.current) return;
		if (isFollowingRef.current) {
			void startFollowing(tail);
		} else {
			void loadTail(tail);
		}
	}, [tail]);

	useEffect(
		() => () => {
			abortRef.current?.abort();
		},
		[],
	);

	// Keep the newest line visible while following.
	useEffect(() => {
		if (isFollowing && scrollRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
	}, [isFollowing]);

	return (
		<VStack align="stretch" spacing={2} data-testid="log-viewer">
			{error ? <ErrorDisplay error={error} /> : null}
			<Box display="flex" gap={4} alignItems="center">
				<Text fontSize="sm">Tail</Text>
				<Select
					size="sm"
					width="auto"
					value={tail}
					onChange={(event) => setTail(Number(event.target.value))}
					data-testid="log-tail-select"
				>
					{TAIL_OPTIONS.map((option) => (
						<option key={option} value={option}>
							last {option} lines
						</option>
					))}
				</Select>
				<Text fontSize="sm">Follow</Text>
				<Switch
					isChecked={isFollowing}
					onChange={onFollowToggle}
					data-testid="log-follow-toggle"
				/>
			</Box>
			<Box
				ref={scrollRef}
				as="pre"
				fontFamily="mono"
				fontSize="sm"
				p={2}
				maxHeight={maxHeight}
				overflowY="auto"
				whiteSpace="pre-wrap"
				data-testid="log-viewer-scroll"
			>
				{lines.length === 0 ? (
					<Text color="gray.500" data-testid="log-viewer-empty">
						No log output.
					</Text>
				) : (
					lines.map((line, index) => (
						// biome-ignore lint/suspicious/noArrayIndexKey: append-only log lines have no stable id; index is their identity
						<Text key={index} data-testid="log-viewer-line">
							{line}
						</Text>
					))
				)}
			</Box>
		</VStack>
	);
}
