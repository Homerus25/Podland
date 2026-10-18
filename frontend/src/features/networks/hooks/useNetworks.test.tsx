import { useNetwork } from "@features/networks/hooks/useNetworks";
import { getNetwork } from "@features/networks/services/networksApi";
import { networkQueryKey } from "@shared/queries/queryKeys";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorResponse } from "@/test/errors";
import { networkDetailFixture } from "@/test/factories";
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

	it("drops the cache entry on unmount (gcTime 0), so every page visit refetches", async () => {
		server.use(
			http.get("*/api/machines/m-1/networks/app-net", () =>
				HttpResponse.json(networkDetailFixture()),
			),
		);

		const { result, queryClient, unmount } = renderHookWithQuery(() =>
			useNetwork("m-1", "app-net"),
		);
		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(
			queryClient.getQueryData(networkQueryKey("m-1", "app-net")),
		).toBeDefined();

		unmount();
		await waitFor(() =>
			expect(
				queryClient.getQueryData(networkQueryKey("m-1", "app-net")),
			).toBeUndefined(),
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
