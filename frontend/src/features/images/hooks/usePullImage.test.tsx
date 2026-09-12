import { usePullImage } from "@features/images/hooks/usePullImage";
import { imagesQueryKey } from "@shared/queries/queryKeys";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { type DefaultBodyType, HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/server";
import { ndjsonResponse, streamFixtures } from "@/test/stream";

type PullResponse = HttpResponse<DefaultBodyType>;
type PullHandler = (request: Request) => PullResponse | Promise<PullResponse>;

const { progressEvent, completeEvent, errorEvent } = streamFixtures;

const pullUrl = "*/api/machines/:machineId/images/pull";

let requests: Request[] = [];

// Routes the pull endpoint through MSW (the repo's single test seam), so the
// real fetch, the AbortController, and the hook's stream handling all run.
function mockPull(handler: PullHandler) {
	requests = [];
	server.use(
		http.post(pullUrl, ({ request }) => {
			requests.push(request);
			return handler(request);
		}),
	);
}

function renderPullHook() {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const wrapper = ({ children }: { children: React.ReactNode }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const hook = renderHook(() => usePullImage("m-1"), { wrapper });
	return { ...hook, queryClient };
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe("usePullImage", () => {
	it("POSTs the image reference and exposes the latest progress event", async () => {
		mockPull(() =>
			ndjsonResponse([
				progressEvent({
					state: { current: 10, total: 100, status: "Downloading layers" },
				}),
				progressEvent({
					state: { current: 50, total: 100, status: "Downloading layers" },
				}),
				completeEvent(),
			]),
		);
		const { result } = renderPullHook();

		await act(async () => {
			result.current.pull("nginx:latest");
		});

		expect(requests).toHaveLength(1);
		expect(requests[0].method).toBe("POST");
		expect(new URL(requests[0].url).pathname).toBe(
			"/api/machines/m-1/images/pull",
		);
		expect(await requests[0].json()).toEqual({ image: "nginx:latest" });
		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(result.current.progress?.state?.current).toBe(50);
		expect(result.current.error).toBeNull();
	});

	it("invalidates the images query on complete", async () => {
		mockPull(() => ndjsonResponse([completeEvent()]));
		const { result, queryClient } = renderPullHook();
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

		await act(async () => {
			result.current.pull("nginx:latest");
		});

		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: imagesQueryKey("m-1"),
		});
	});

	it("routes the error envelope out of the stream", async () => {
		mockPull(() =>
			ndjsonResponse([errorEvent("REGISTRY_UNREACHABLE", "Registry is down")]),
		);
		const { result } = renderPullHook();

		await act(async () => {
			result.current.pull("nginx:latest");
		});

		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(result.current.error).toEqual({
			code: "REGISTRY_UNREACHABLE",
			message: "Registry is down",
		});
	});

	it("exposes non-2xx HTTP failures through the error envelope", async () => {
		mockPull(() =>
			HttpResponse.json(
				{
					requestId: "req-1",
					error: { code: "MACHINE_NOT_FOUND", message: "Machine not found" },
				},
				{ status: 404 },
			),
		);
		const { result } = renderPullHook();

		await act(async () => {
			result.current.pull("nginx:latest");
		});

		await waitFor(() => expect(result.current.isPulling).toBe(false));
		expect(result.current.error).toEqual({
			code: "MACHINE_NOT_FOUND",
			message: "Machine not found",
		});
	});
});
