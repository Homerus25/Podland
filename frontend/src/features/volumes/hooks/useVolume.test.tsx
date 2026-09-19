import { useVolume } from "@features/volumes/hooks/useVolume";
import { inspectVolume } from "@features/volumes/services/volumesApi";
import { volumeQueryKey } from "@shared/queries/queryKeys";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { volumeFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

describe("inspectVolume", () => {
	it("fetches a single volume through the API", async () => {
		const volume = volumeFixture();
		server.use(
			http.get("*/api/machines/m-1/volumes/app-data", () =>
				HttpResponse.json(volume),
			),
		);

		await expect(inspectVolume("m-1", "app-data")).resolves.toEqual(volume);
	});
});

describe("useVolume", () => {
	it("returns the volume's full inspect data", async () => {
		const volume = volumeFixture();
		server.use(
			http.get("*/api/machines/m-1/volumes/app-data", () =>
				HttpResponse.json(volume),
			),
		);

		const { result, queryClient } = renderHookWithQuery(() =>
			useVolume("m-1", "app-data"),
		);
		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toEqual(volume);

		expect(queryClient.getQueryData(volumeQueryKey("m-1", "app-data"))).toEqual(
			volume,
		);
	});

	it("does not fetch when no volume is selected", () => {
		const { result } = renderHookWithQuery(() => useVolume("m-1", null));
		expect(result.current.isPending).toBe(true);
		expect(result.current.fetchStatus).toBe("idle");
	});

	it("exposes 404 without retrying when the volume is gone", async () => {
		server.use(
			http.get("*/api/machines/m-1/volumes/gone", () =>
				HttpResponse.json(
					{
						requestId: "r-1",
						error: { code: "VOLUME_NOT_FOUND", message: "Volume not found" },
					},
					{ status: 404 },
				),
			),
		);

		const { result } = renderHookWithQuery(() => useVolume("m-1", "gone"));
		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(
			(result.current.error as { response?: { status?: number } })?.response
				?.status,
		).toBe(404);
	});
});
