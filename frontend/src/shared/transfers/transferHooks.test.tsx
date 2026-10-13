import {
	volumeMovePreviewQueryKey,
	volumesQueryKey,
} from "@shared/queries/queryKeys";
import {
	invalidateMoveResult,
	type MoveStreamConfig,
	useMovePreview,
	useMoveStream,
} from "@shared/transfers/transferHooks";
import { api, machineApiPath } from "@shared/utils/api";
import { QueryClient } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { abortableStream, ndjsonResponse, streamResponse } from "@/test/stream";

afterEach(() => {
	vi.restoreAllMocks();
});

// The transfer-hooks module's suite (issue 273): the ritual every
// cross-machine move/copy surface shares is pinned once here — the
// null-target preview, the both-machines invalidation (a partial result is
// success-shaped), the terminal handling, the malformed-event skip, and the
// abort-never-an-error rule. The suite supplies its own minimal surface (a
// key constructor, fetcher, URL builder, event vocabulary, shape guard, and
// fold) because the module imports no domain types; the feature suites keep
// their domain assertions only.

// The suite's own move-stream surface, standing in for a feature's: one
// progress event kind, then exactly one terminal event — the transfer
// envelope on "complete", the mapped code/message on "error".
interface TestMoveResult {
	name: string;
	status: "moved" | "partial";
	sourceMachineId: string;
	targetMachineId: string;
}

interface TestMoveEvent {
	operationId: string;
	type: "progress" | "complete" | "error";
	machineId: string;
	phase?: string;
	result?: TestMoveResult;
	error?: { code: string; message: string };
}

function isTestMoveEvent(value: unknown): value is TestMoveEvent {
	if (typeof value !== "object" || value === null) return false;
	const event = value as Record<string, unknown>;
	return (
		typeof event.operationId === "string" &&
		(event.type === "progress" ||
			event.type === "complete" ||
			event.type === "error")
	);
}

// The single-value fold: the latest progress event wins outright, so the
// previous state plays no part.
function foldProgress(event: TestMoveEvent): string | null {
	return event.phase ?? "start";
}

const streamConfig: MoveStreamConfig<TestMoveEvent, string | null> = {
	resource: "volumes",
	url: (id) => `/api/machines/m-1/things/${id}/move`,
	isEvent: isTestMoveEvent,
	foldProgress,
	initialProgress: null,
};

function renderMoveStream() {
	return renderHookWithQuery(() =>
		useMoveStream<TestMoveEvent, string | null, TestMoveResult>(streamConfig),
	);
}

function testMoveResult(
	overrides: Partial<TestMoveResult> = {},
): TestMoveResult {
	return {
		name: "app",
		status: "moved",
		sourceMachineId: "m-1",
		targetMachineId: "m-2",
		...overrides,
	};
}

function progressEvent(overrides: Record<string, unknown> = {}) {
	return {
		operationId: "op-1",
		type: "progress",
		machineId: "m-1",
		phase: "start",
		...overrides,
	};
}

function completeEvent(result: TestMoveResult) {
	return { operationId: "op-1", type: "complete", machineId: "m-1", result };
}

function errorEvent(code: string, message: string) {
	return {
		operationId: "op-1",
		type: "error",
		machineId: "m-1",
		error: { code, message },
	};
}

// The events travel as raw framed lines: the suite's event vocabulary is
// its own, not the shared stream kit's typed fixtures.
function moveStreamResponse(events: Record<string, unknown>[]) {
	return ndjsonResponse(
		[],
		events.map((event) => `${JSON.stringify(event)}\n`),
	);
}

type MoveResponse = HttpResponse<DefaultBodyType>;
type MoveHandler = (request: Request) => MoveResponse | Promise<MoveResponse>;

const moveUrl = "*/api/machines/m-1/things/app/move";

let requests: Request[] = [];

function mockMoveStream(handler: MoveHandler) {
	requests = [];
	server.use(
		http.post(moveUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

const transferInput = { targetMachineId: "m-2", mode: "move" as const };

describe("useMoveStream", () => {
	it("POSTs the transfer body to the surface's URL as an NDJSON operation", async () => {
		mockMoveStream(() => moveStreamResponse([completeEvent(testMoveResult())]));
		const { result } = renderMoveStream();

		await act(async () => {
			result.current.run("app", transferInput);
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			"/api/machines/m-1/things/app/move",
		);
		expect(requests[0].headers.get("Content-Type")).toBe("application/json");
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(await requests[0].json()).toEqual(transferInput);
		expect(result.current.result?.status).toBe("moved");
		expect(result.current.error).toBeNull();
	});

	it("routes guarded progress events through the surface's fold", async () => {
		mockMoveStream(() =>
			moveStreamResponse([
				progressEvent({ phase: "start" }),
				progressEvent({ phase: "bytes" }),
				completeEvent(testMoveResult()),
			]),
		);
		const { result } = renderMoveStream();

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.progress).toBe("bytes");
	});

	it("resets the progress and result on each run", async () => {
		mockMoveStream(() =>
			moveStreamResponse([
				progressEvent({ phase: "bytes" }),
				completeEvent(testMoveResult({ name: "first" })),
			]),
		);
		const { result } = renderMoveStream();

		await act(async () => {
			result.current.run("app", transferInput);
		});
		await waitFor(() => expect(result.current.result?.name).toBe("first"));
		expect(result.current.progress).toBe("bytes");

		// The second run's stream carries no progress event, so the reset is
		// what leaves the progress at its initial state.
		mockMoveStream(() =>
			moveStreamResponse([completeEvent(testMoveResult({ name: "second" }))]),
		);
		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.result?.name).toBe("second"));
		expect(result.current.progress).toBeNull();
	});

	it("lands the terminal complete event's envelope in the result and invalidates both machines' lists", async () => {
		mockMoveStream(() => moveStreamResponse([completeEvent(testMoveResult())]));
		const { result, queryClient } = renderMoveStream();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.result).toEqual(testMoveResult());
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-2"),
		});
	});

	it("invalidates both machines on a partial result too — the target install stands", async () => {
		mockMoveStream(() =>
			moveStreamResponse([
				completeEvent(testMoveResult({ status: "partial" })),
			]),
		);
		const { result, queryClient } = renderMoveStream();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.result?.status).toBe("partial");
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-2"),
		});
	});

	it("warns and skips a complete event without a transfer envelope", async () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
		const envelopeLess = {
			operationId: "op-1",
			type: "complete",
			machineId: "m-1",
		};
		mockMoveStream(() => moveStreamResponse([envelopeLess]));
		const { result, queryClient } = renderMoveStream();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(warn).toHaveBeenCalledWith(
			"Skipping complete event without a transfer envelope:",
			envelopeLess,
		);
		expect(result.current.result).toBeNull();
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("warns and skips a malformed event, folding only guarded ones", async () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
		// No operationId: outside the surface's event vocabulary.
		const malformed = { type: "progress", phase: "start" };
		mockMoveStream(() =>
			moveStreamResponse([
				malformed,
				progressEvent({ phase: "bytes" }),
				completeEvent(testMoveResult()),
			]),
		);
		const { result } = renderMoveStream();

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(warn).toHaveBeenCalledWith(
			"Skipping malformed stream event (unexpected shape):",
			malformed,
		);
		expect(result.current.progress).toBe("bytes");
		expect(result.current.result?.status).toBe("moved");
	});

	it("surfaces a terminal error event with its code and message, without a result or invalidation", async () => {
		mockMoveStream(() =>
			moveStreamResponse([
				progressEvent(),
				errorEvent("THING_IN_USE", "thing is in use"),
			]),
		);
		const { result, queryClient } = renderMoveStream();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.run("app", transferInput);
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toEqual({
			code: "THING_IN_USE",
			message: "thing is in use",
		});
		// The error terminal is the transport's: it is not folded into progress.
		expect(result.current.progress).toBe("start");
		expect(result.current.result).toBeNull();
		expect(invalidateSpy).not.toHaveBeenCalled();
	});

	it("never surfaces an abort as an error", async () => {
		let requestSignal: AbortSignal | undefined;
		mockMoveStream((request) => {
			requestSignal = request.signal;
			return streamResponse(
				abortableStream(
					[`${JSON.stringify(progressEvent())}\n`],
					request.signal,
				),
			);
		});
		const { result } = renderMoveStream();

		act(() => {
			result.current.run("app", transferInput);
		});
		await waitFor(() => expect(result.current.progress).toBe("start"));

		act(() => {
			result.current.cancel();
		});

		expect(requestSignal?.aborted).toBe(true);
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toBeNull();
	});
});

// The suite's preview surface: the shared key vocabulary's volume preview
// constructor and a fetcher over the shared axios instance.
interface TestPreview {
	collision: boolean;
}

async function fetchTestPreview(targetMachineId: string): Promise<TestPreview> {
	const response = await api.get<TestPreview>(
		machineApiPath("m-1", "volumes", "app-data", "move-preview"),
		{ params: { targetMachineId } },
	);
	return response.data;
}

function renderMovePreview(targetMachineId: string | null) {
	return renderHookWithQuery(() =>
		useMovePreview(
			(target) => volumeMovePreviewQueryKey("m-1", "app-data", target),
			fetchTestPreview,
			targetMachineId,
		),
	);
}

describe("useMovePreview", () => {
	it("stays disabled and fetches nothing while no target is chosen, keyed by the empty-string placeholder", () => {
		let fetches = 0;
		server.use(
			http.get("*/api/machines/m-1/volumes/app-data/move-preview", () => {
				fetches++;
				return HttpResponse.json({ collision: false });
			}),
		);
		const { result, queryClient } = renderMovePreview(null);

		expect(result.current.fetchStatus).toBe("idle");
		expect(result.current.status).toBe("pending");
		expect(fetches).toBe(0);
		expect(
			queryClient
				.getQueryCache()
				.getAll()
				.map((query) => query.queryKey),
		).toContainEqual(volumeMovePreviewQueryKey("m-1", "app-data", ""));
	});

	it("fetches through the surface's fetcher once a target is chosen, keyed by the target", async () => {
		const targets: string[] = [];
		server.use(
			http.get(
				"*/api/machines/m-1/volumes/app-data/move-preview",
				({ request }) => {
					targets.push(
						new URL(request.url).searchParams.get("targetMachineId") ?? "",
					);
					return HttpResponse.json({ collision: false });
				},
			),
		);
		const { result, queryClient } = renderMovePreview("m-2");

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(targets).toEqual(["m-2"]);
		expect(result.current.data).toEqual({ collision: false });
		expect(
			queryClient
				.getQueryCache()
				.getAll()
				.map((query) => query.queryKey),
		).toContainEqual(volumeMovePreviewQueryKey("m-1", "app-data", "m-2"));
	});
});

describe("invalidateMoveResult", () => {
	it("invalidates the resource's lists on both machines from the envelope's machine ids", () => {
		const queryClient = new QueryClient();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		invalidateMoveResult(
			queryClient,
			{ sourceMachineId: "m-1", targetMachineId: "m-2" },
			"volumes",
		);

		expect(invalidateSpy).toHaveBeenCalledTimes(2);
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-1"),
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: volumesQueryKey("m-2"),
		});
	});
});
