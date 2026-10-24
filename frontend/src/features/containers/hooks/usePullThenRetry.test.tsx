import { TextEncoder } from "node:util";
import { usePullThenRetry } from "@features/containers/hooks/usePullThenRetry";
import { act, waitFor } from "@testing-library/react";
import { type DefaultBodyType, type HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";
import { ndjsonResponse, streamFixtures, streamResponse } from "@/test/stream";

// The pull-then-retry-once invariant (spec #371, ticket #370), pinned at
// the hook's public seam: the hook test renderer plus the MSW-backed
// images pull stream. Requesting a pull arms a synchronous ref guard; a
// pull that settles without an error fires the caller's post-pull
// callback exactly once — under interleaved re-renders and callback
// identity churn — and a pull that settles with an error, or no pull at
// all, fires nothing.

const { progressEvent, completeEvent, errorEvent } = streamFixtures;

const pullUrl = "*/api/machines/m-1/images/pull";

let requestCount = 0;

function mockPullStream(response: HttpResponse<DefaultBodyType>) {
	requestCount = 0;
	server.use(
		http.post(pullUrl, () => {
			requestCount++;
			return response;
		}),
	);
}

// A pull stream that emits its progress event and then stays open until
// release() appends the terminal event, so a test can interleave
// re-renders mid-pull deterministically instead of racing a delayed
// stream.
function heldPullStream() {
	let release: () => void = () => {};
	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			const encoder = new TextEncoder();
			controller.enqueue(
				encoder.encode(`${JSON.stringify(progressEvent())}\n`),
			);
			release = () => {
				controller.enqueue(
					encoder.encode(`${JSON.stringify(completeEvent())}\n`),
				);
				controller.close();
			};
		},
	});
	return { response: streamResponse(stream), release: () => release() };
}

describe("usePullThenRetry", () => {
	it("fires the post-pull callback exactly once when the requested pull settles without an error", async () => {
		mockPullStream(ndjsonResponse([progressEvent(), completeEvent()]));
		const onPulled = vi.fn();
		const { result } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", onPulled),
		);

		expect(onPulled).not.toHaveBeenCalled();
		await act(async () => {
			result.current.startPull("nginx:latest");
		});

		await waitFor(() => expect(result.current.isPulling).toBe(false));
		await waitFor(() => expect(onPulled).toHaveBeenCalledTimes(1));
		expect(requestCount).toBe(1);
	});

	it("fires nothing without a requested pull, across re-renders", async () => {
		mockPullStream(ndjsonResponse([completeEvent()]));
		const onPulled = vi.fn();
		const { rerender } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", onPulled),
		);

		await act(async () => {
			rerender();
			rerender();
		});

		expect(onPulled).not.toHaveBeenCalled();
		expect(requestCount).toBe(0);
	});

	it("fires nothing when the pull settles with an error", async () => {
		mockPullStream(
			ndjsonResponse([errorEvent("REGISTRY_UNREACHABLE", "Registry is down")]),
		);
		const onPulled = vi.fn();
		const { result } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", onPulled),
		);

		await act(async () => {
			result.current.startPull("nginx:latest");
		});

		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(result.current.error).toEqual({
			code: "REGISTRY_UNREACHABLE",
			message: "Registry is down",
		});
		expect(onPulled).not.toHaveBeenCalled();
	});

	it("is re-render-storm-proof: interleaved re-renders during and after the pull neither double-fire nor lose the fire", async () => {
		const held = heldPullStream();
		mockPullStream(held.response);
		const onPulled = vi.fn();
		const { result, rerender } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", onPulled),
		);

		await act(async () => {
			result.current.startPull("nginx:latest");
		});
		await waitFor(() => expect(result.current.isPulling).toBe(true));

		// Interleaved re-renders mid-pull: no premature fire.
		await act(async () => {
			rerender();
			rerender();
			rerender();
		});
		expect(onPulled).not.toHaveBeenCalled();

		await act(async () => {
			held.release();
		});
		await waitFor(() => expect(result.current.isPulling).toBe(false));
		await waitFor(() => expect(onPulled).toHaveBeenCalledTimes(1));

		// Interleaved re-renders after the fire: no second fire.
		await act(async () => {
			rerender();
			rerender();
			rerender();
		});
		expect(onPulled).toHaveBeenCalledTimes(1);
	});

	it("holds the latest callback through identity churn without re-triggering the fire", async () => {
		const held = heldPullStream();
		mockPullStream(held.response);
		const first = vi.fn();
		const second = vi.fn();
		let current = first;
		const { result, rerender } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", current),
		);

		await act(async () => {
			result.current.startPull("nginx:latest");
		});
		await waitFor(() => expect(result.current.isPulling).toBe(true));

		// Identity churn mid-pull: the hook holds the latest callback and
		// the churn cannot re-trigger the effect.
		current = second;
		await act(async () => {
			rerender();
			rerender();
		});

		await act(async () => {
			held.release();
		});
		await waitFor(() => expect(result.current.isPulling).toBe(false));
		await waitFor(() => expect(second).toHaveBeenCalledTimes(1));
		expect(first).not.toHaveBeenCalled();

		// Churn after the fire cannot re-trigger it.
		const third = vi.fn();
		current = third;
		await act(async () => {
			rerender();
			rerender();
		});
		expect(second).toHaveBeenCalledTimes(1);
		expect(third).not.toHaveBeenCalled();
	});

	it("re-arms after a failed pull: a second requested pull that succeeds fires exactly once", async () => {
		const held = heldPullStream();
		// The first requested pull fails; the stream stays open for the
		// second request, which the handler answers with success.
		let failFirst = true;
		server.use(
			http.post(pullUrl, () => {
				requestCount++;
				if (failFirst) {
					failFirst = false;
					return ndjsonResponse([
						errorEvent("REGISTRY_UNREACHABLE", "Registry is down"),
					]);
				}
				return held.response;
			}),
		);
		requestCount = 0;
		const onPulled = vi.fn();
		const { result } = renderHookWithQuery(() =>
			usePullThenRetry("m-1", onPulled),
		);

		await act(async () => {
			result.current.startPull("nginx:latest");
		});
		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(onPulled).not.toHaveBeenCalled();

		await act(async () => {
			result.current.startPull("nginx:latest");
		});
		await waitFor(() => expect(result.current.isPulling).toBe(true));
		await act(async () => {
			held.release();
		});
		await waitFor(() => expect(result.current.isPulling).toBe(false));
		await waitFor(() => expect(onPulled).toHaveBeenCalledTimes(1));
		expect(requestCount).toBe(2);
	});
});
