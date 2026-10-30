import { useContainers } from "@features/containers/hooks/useContainers";
import {
	ConnectError,
	useConnectContainers,
	useNetwork,
	useNetworks,
} from "@features/networks/hooks/useNetworks";
import { getNetwork } from "@features/networks/services/networksApi";
import { networkQueryKey } from "@shared/queries/queryKeys";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http, type JsonBodyType } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import {
	containerFixture,
	networkDetailFixture,
	networkFixture,
} from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

describe("getNetwork", () => {
	it("fetches a single network's detail through the API", async () => {
		const detail = networkDetailFixture();
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", () =>
				HttpResponse.json(detail),
			),
		);

		await expect(getNetwork("m-1", "app-net")).resolves.toEqual(detail);
	});
});

describe("useNetwork", () => {
	it("returns the network's detail cached under the detail query key", async () => {
		const detail = networkDetailFixture();
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", () =>
				HttpResponse.json(detail),
			),
		);

		const { result, queryClient } = renderHookWithQuery(() =>
			useNetwork("m-1", "app-net"),
		);
		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toEqual(detail);
		expect(queryClient.getQueryData(networkQueryKey("m-1", "app-net"))).toEqual(
			detail,
		);
	});

	it("exposes 404 without retrying when the network is gone", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks/gone", () =>
				errorResponse(404, "NETWORK_NOT_FOUND", "Network not found", "r-1"),
			),
		);

		const { result } = renderHookWithQuery(() => useNetwork("m-1", "gone"));
		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(
			(result.current.error as { response?: { status?: number } })?.response
				?.status,
		).toBe(404);
	});
});

// --- issue 320: multi-connect --------------------------------------------

let networksGets = 0;
let containersGets = 0;
let connectRequests: { url: string; body: unknown }[] = [];

function resetConnectCounters() {
	networksGets = 0;
	containersGets = 0;
	connectRequests = [];
}

function mockConnectLists() {
	return [
		http.get("*/api/machines/m-1/networks", () => {
			networksGets++;
			return HttpResponse.json([networkFixture()]);
		}),
		http.get("*/api/machines/m-1/containers", () => {
			containersGets++;
			return HttpResponse.json({
				containers: [containerFixture()],
				total: 1,
				timestamp: "2026-08-26T00:00:00Z",
			});
		}),
	];
}

// failures maps "networkName/containerId" to the error response that pair
// always answers with; every other pair connects successfully.
function connectHandler(
	failures: Record<string, HttpResponse<JsonBodyType>> = {},
) {
	return http.post(
		"*/api/machines/m-1/networks/:networkName/connect",
		async ({ request, params }) => {
			const body = (await request.json()) as { container: string };
			connectRequests.push({ url: request.url, body });
			const failure = failures[`${params.networkName}/${body.container}`];
			if (failure) return failure;
			return new HttpResponse(null, { status: 200 });
		},
	);
}

describe("useConnectContainers", () => {
	it("fires one connect request per pair and invalidates both lists once after all settle", async () => {
		resetConnectCounters();
		server.use(...mockConnectLists(), connectHandler());
		// The list queries ride the same hook render so the invalidation's
		// refetch is observable through MSW, not an invalidation spy.
		const { result } = renderHookWithQuery(() => {
			useNetworks("m-1");
			useContainers("m-1");
			return useConnectContainers("m-1");
		});
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));

		result.current.mutate([
			{ networkName: "app-net", containerId: "c-2" },
			{ networkName: "app-net", containerId: "c-4" },
		]);

		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(connectRequests).toHaveLength(2);
		expect(connectRequests.map((request) => request.body)).toEqual(
			expect.arrayContaining([{ container: "c-2" }, { container: "c-4" }]),
		);
		for (const request of connectRequests) {
			expect(
				request.url.endsWith("/api/machines/m-1/networks/app-net/connect"),
			).toBe(true);
		}
		// One invalidation round after settle — not one per request.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
	});

	it("fires every request and aggregates the failures when a pair fails, still invalidating both lists", async () => {
		resetConnectCounters();
		server.use(
			...mockConnectLists(),
			connectHandler({
				"app-net/c-2": errorResponse(
					404,
					"CONTAINER_NOT_FOUND",
					"container not found",
					"req-11",
				),
			}),
		);
		const { result } = renderHookWithQuery(() => {
			useNetworks("m-1");
			useContainers("m-1");
			return useConnectContainers("m-1");
		});
		await waitFor(() => expect(containersGets).toBe(1));
		await waitFor(() => expect(networksGets).toBe(1));

		result.current.mutate([
			{ networkName: "app-net", containerId: "c-2" },
			{ networkName: "app-net", containerId: "c-4" },
		]);

		await waitFor(() => expect(result.current.isError).toBe(true));
		// All requests fire regardless of the individual failure.
		expect(connectRequests).toHaveLength(2);
		const error = result.current.error;
		expect(error).toBeInstanceOf(ConnectError);
		expect(error?.failures).toEqual([
			{
				networkName: "app-net",
				containerId: "c-2",
				error: { code: "CONTAINER_NOT_FOUND", message: "container not found" },
			},
		]);
		// The settle still invalidates both lists, so succeeded items leave
		// the eligible options and failed ones remain for retry.
		await waitFor(() => expect(containersGets).toBe(2));
		await waitFor(() => expect(networksGets).toBe(2));
	});
});
