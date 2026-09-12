import {
	streamNdjson,
	useNdjsonOperation,
} from "@shared/streaming/streamTransport";
import { genericError, setBearerToken } from "@shared/utils/api";
import { act, renderHook, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";
import {
	abortableNdjsonResponse,
	ndjsonResponse,
	streamFixtures,
	streamResponse,
} from "@/test/stream";

const { progressEvent, completeEvent, errorEvent } = streamFixtures;

const streamUrl = "*/api/transport-stream";

type StreamHandler = (request: Request) => Response | Promise<Response>;

let requests: Request[] = [];

function mockStream(method: "get" | "post", handler: StreamHandler) {
	requests = [];
	server.use(
		http[method](streamUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

afterEach(() => {
	setBearerToken(null);
	vi.restoreAllMocks();
});

describe("streamNdjson", () => {
	it("sends the Bearer header when a token is held and omits it when none is", async () => {
		mockStream("post", () => ndjsonResponse([completeEvent()]));
		const values: unknown[] = [];

		setBearerToken("token-1");
		await streamNdjson(
			"/api/transport-stream",
			{ method: "POST", body: "{}", signal: new AbortController().signal },
			(value) => values.push(value),
		);
		expect(requests[0].headers.get("Authorization")).toBe("Bearer token-1");

		setBearerToken(null);
		await streamNdjson(
			"/api/transport-stream",
			{ method: "POST", body: "{}", signal: new AbortController().signal },
			(value) => values.push(value),
		);
		expect(requests[1].headers.get("Authorization")).toBeNull();
	});

	it("sends Content-Type only when a body is present", async () => {
		mockStream("post", () => ndjsonResponse([completeEvent()]));
		server.use(
			http.get(streamUrl, ({ request }) => {
				requests.push(request);
				return ndjsonResponse([completeEvent()]);
			}),
		);

		await streamNdjson(
			"/api/transport-stream",
			{ method: "POST", body: "{}", signal: new AbortController().signal },
			() => undefined,
		);
		await streamNdjson(
			"/api/transport-stream",
			{ signal: new AbortController().signal },
			() => undefined,
		);
		await streamNdjson(
			"/api/transport-stream",
			{ method: "POST", signal: new AbortController().signal },
			() => undefined,
		);

		expect(requests[0].headers.get("Content-Type")).toBe("application/json");
		expect(requests[1].headers.get("Content-Type")).toBeNull();
		expect(requests[2].headers.get("Content-Type")).toBeNull();
	});

	it("rejects non-ok responses with the envelope, collapsing internal codes to the generic sentinel", async () => {
		mockStream("post", () =>
			HttpResponse.json(
				{
					requestId: "req-1",
					error: { code: "MACHINE_NOT_FOUND", message: "Machine not found" },
				},
				{ status: 404 },
			),
		);
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				() => undefined,
			),
		).rejects.toEqual({
			code: "MACHINE_NOT_FOUND",
			message: "Machine not found",
		});

		mockStream("post", () =>
			HttpResponse.json(
				{
					requestId: "req-2",
					error: {
						code: "SERVICE_UNAVAILABLE",
						message: "internal detail",
					},
				},
				{ status: 404 },
			),
		);
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				() => undefined,
			),
		).rejects.toEqual(genericError);
	});

	it("rejects a response without a body with the generic sentinel and resolves on clean stream end", async () => {
		mockStream("post", () => new Response(null, { status: 200 }));
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				() => undefined,
			),
		).rejects.toEqual(genericError);

		mockStream("post", () => ndjsonResponse([completeEvent()]));
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				() => undefined,
			),
		).resolves.toBeUndefined();
	});

	it("rethrows abort errors raw, from the fetch and from the stream read", async () => {
		const preAborted = new AbortController();
		preAborted.abort();
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", body: "{}", signal: preAborted.signal },
				() => undefined,
			),
		).rejects.toMatchObject({ name: "AbortError" });

		let requestSignal: AbortSignal | undefined;
		mockStream("post", (request) => {
			requestSignal = request.signal;
			return abortableNdjsonResponse([progressEvent()], request.signal);
		});
		const controller = new AbortController();
		const values: unknown[] = [];
		const consumption = streamNdjson(
			"/api/transport-stream",
			{ method: "POST", signal: controller.signal },
			(value) => values.push(value),
		);
		await waitFor(() => expect(values).toHaveLength(1));
		controller.abort();
		expect(requestSignal?.aborted).toBe(true);
		await expect(consumption).rejects.toMatchObject({ name: "AbortError" });
	});

	it("collapses non-abort fetch and read failures into the generic sentinel", async () => {
		mockStream("post", () => {
			throw new Error("boom");
		});
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				() => undefined,
			),
		).rejects.toEqual(genericError);

		mockStream("post", () =>
			streamResponse(
				new ReadableStream<Uint8Array>({
					start(controller) {
						controller.enqueue(
							new TextEncoder().encode(`${JSON.stringify(progressEvent())}\n`),
						);
						setTimeout(() => {
							controller.error(new Error("stream boom"));
						}, 10);
					},
				}),
			),
		);
		const values: unknown[] = [];
		await expect(
			streamNdjson(
				"/api/transport-stream",
				{ method: "POST", signal: new AbortController().signal },
				(value) => values.push(value),
			),
		).rejects.toEqual(genericError);
		expect(values).toHaveLength(1);
	});

	it("delivers every parsed value, warning and skipping malformed lines and reassembling split events", async () => {
		const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
		const complete = JSON.stringify(completeEvent());
		mockStream("post", () =>
			ndjsonResponse(
				[],
				[
					`${JSON.stringify({ operationId: "op-1", type: "log", line: "hi" })}\n`,
					"garbage\n",
					complete.slice(0, 10),
					`${complete.slice(10)}\n`,
				],
			),
		);
		const values: unknown[] = [];
		await streamNdjson(
			"/api/transport-stream",
			{ method: "POST", signal: new AbortController().signal },
			(value) => values.push(value),
		);

		expect(values).toEqual([
			{ operationId: "op-1", type: "log", line: "hi" },
			expect.objectContaining({ type: "complete" }),
		]);
		expect(warn).toHaveBeenCalledTimes(1);
	});
});

function renderOperation() {
	return renderHook(() => useNdjsonOperation());
}

describe("useNdjsonOperation", () => {
	it("returns the operation contract and keeps start fire-and-forget", async () => {
		let release: (() => void) | undefined;
		mockStream("post", async () => {
			await new Promise<void>((resolve) => {
				release = resolve;
			});
			return ndjsonResponse([completeEvent()]);
		});
		const { result } = renderOperation();

		let startResult: unknown;
		act(() => {
			startResult = result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: () => undefined,
			});
		});

		expect(startResult).toBeUndefined();
		expect(result.current.isRunning).toBe(true);
		expect(result.current.error).toBeNull();

		await waitFor(() => expect(requests).toHaveLength(1));
		await act(async () => {
			release?.();
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
	});

	it("routes every value through onEvent and ends the run exactly once on complete", async () => {
		mockStream("post", () =>
			ndjsonResponse([progressEvent(), completeEvent()]),
		);
		const { result } = renderOperation();
		const events: unknown[] = [];

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: (event) => events.push(event),
			});
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(events).toHaveLength(2);
		expect(events[0]).toMatchObject({ type: "progress" });
		expect(events[1]).toMatchObject({ type: "complete" });
		expect(result.current.error).toBeNull();
	});

	it("finishes on an error event with its code and message, generic when absent", async () => {
		mockStream("get", () =>
			ndjsonResponse([errorEvent("SSH_FAILED", "Machine unreachable")]),
		);
		const { result } = renderOperation();
		const events: unknown[] = [];

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				onEvent: (event) => events.push(event),
			});
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(events).toHaveLength(1);
		expect(result.current.error).toEqual({
			code: "SSH_FAILED",
			message: "Machine unreachable",
		});

		mockStream("get", () =>
			ndjsonResponse(
				[],
				[`${JSON.stringify({ operationId: "op-1", type: "error" })}\n`],
			),
		);
		const second = renderOperation();
		act(() => {
			second.result.current.start({
				url: "/api/transport-stream",
				onEvent: () => undefined,
			});
		});
		await waitFor(() => expect(second.result.current.isRunning).toBe(false));
		expect(second.result.current.error).toEqual(genericError);
	});

	it("ignores a second terminal event after the run finished", async () => {
		mockStream("get", () =>
			ndjsonResponse([completeEvent(), errorEvent("LATE", "too late")]),
		);
		const { result } = renderOperation();
		const events: unknown[] = [];

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				onEvent: (event) => events.push(event),
			});
		});

		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(events).toHaveLength(1);
		expect(result.current.error).toBeNull();
	});

	it("aborts the previous run on a new start; its late events reach nobody", async () => {
		const encoder = new TextEncoder();
		let pushFirst: ((chunk: string) => void) | undefined;
		let closeFirst: (() => void) | undefined;
		requests = [];
		server.use(
			http.post(streamUrl, ({ request }) => {
				requests.push(request);
				if (requests.length === 1) {
					return streamResponse(
						new ReadableStream<Uint8Array>({
							start(controller) {
								controller.enqueue(
									encoder.encode(`${JSON.stringify(progressEvent())}\n`),
								);
								pushFirst = (chunk) =>
									controller.enqueue(encoder.encode(chunk));
								closeFirst = () => controller.close();
							},
						}),
					);
				}
				return ndjsonResponse([completeEvent()]);
			}),
		);
		const { result } = renderOperation();
		const events: unknown[] = [];

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: (event) => events.push(event),
			});
		});
		await waitFor(() => expect(events).toHaveLength(1));
		expect(result.current.isRunning).toBe(true);

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: (event) => events.push(event),
			});
		});
		expect(requests[0].signal.aborted).toBe(true);

		// Whether the superseded body survives the request abort is the
		// platform's call; if it delivers, its late terminal must reach nobody.
		await act(async () => {
			try {
				pushFirst?.(`${JSON.stringify(completeEvent())}\n`);
				closeFirst?.();
			} catch {
				// the superseded stream was already torn down
			}
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(events).toHaveLength(2);
		expect(events[1]).toMatchObject({ type: "complete" });
		expect(result.current.error).toBeNull();
	});

	it("cancel aborts the in-flight run and leaves error null", async () => {
		let requestSignal: AbortSignal | undefined;
		mockStream("post", (request) => {
			requestSignal = request.signal;
			return abortableNdjsonResponse([progressEvent()], request.signal);
		});
		const { result } = renderOperation();
		const events: unknown[] = [];

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: (event) => events.push(event),
			});
		});
		await waitFor(() => expect(events).toHaveLength(1));

		act(() => {
			result.current.cancel();
		});

		expect(requestSignal?.aborted).toBe(true);
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toBeNull();
	});

	it("surfaces transport failures as the error state and clears it on the next start", async () => {
		mockStream("post", () =>
			HttpResponse.json(
				{
					requestId: "req-1",
					error: { code: "MACHINE_NOT_FOUND", message: "Machine not found" },
				},
				{ status: 404 },
			),
		);
		const { result } = renderOperation();

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: () => undefined,
			});
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toEqual({
			code: "MACHINE_NOT_FOUND",
			message: "Machine not found",
		});

		let release: (() => void) | undefined;
		mockStream("post", async () => {
			await new Promise<void>((resolve) => {
				release = resolve;
			});
			return ndjsonResponse([completeEvent()]);
		});
		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: () => undefined,
			});
		});
		await waitFor(() => expect(requests).toHaveLength(1));
		expect(result.current.error).toBeNull();
		expect(result.current.isRunning).toBe(true);
		await act(async () => {
			release?.();
		});
		await waitFor(() => expect(result.current.isRunning).toBe(false));
		expect(result.current.error).toBeNull();
	});

	it("aborts the in-flight run on unmount", async () => {
		let requestSignal: AbortSignal | undefined;
		let release: (() => void) | undefined;
		mockStream("post", async (request) => {
			requestSignal = request.signal;
			await new Promise<void>((resolve) => {
				release = resolve;
			});
			return ndjsonResponse([completeEvent()]);
		});
		const { result, unmount } = renderOperation();

		act(() => {
			result.current.start({
				url: "/api/transport-stream",
				init: { method: "POST", body: "{}" },
				onEvent: () => undefined,
			});
		});
		await waitFor(() => expect(requests).toHaveLength(1));

		unmount();
		expect(requestSignal?.aborted).toBe(true);
		await act(async () => {
			release?.();
		});
	});
});
