import { useVolumes } from "@features/volumes/hooks/useVolumes";
import { listVolumes } from "@features/volumes/services/volumesApi";
import { volumesQueryKey } from "@shared/queries/queryKeys";
import { waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { volumeFixture } from "@/test/factories";
import { renderHookWithQuery } from "@/test/render";
import { server } from "@/test/server";

describe("listVolumes", () => {
	it("fetches the machine volumes through the API", async () => {
		const volumes = [volumeFixture()];
		server.use(
			http.get("*/api/machines/m-1/volumes", () => HttpResponse.json(volumes)),
		);

		await expect(listVolumes("m-1")).resolves.toEqual(volumes);
	});
});

describe("useVolumes", () => {
	it("returns and caches the volume list per machine", async () => {
		const volumes = [
			volumeFixture(),
			volumeFixture({ name: "tmp", anonymous: true, size: 4096 }),
		];
		server.use(
			http.get("*/api/machines/m-1/volumes", () => HttpResponse.json(volumes)),
		);

		const { result, queryClient } = renderHookWithQuery(() =>
			useVolumes("m-1"),
		);
		await waitFor(() => expect(result.current.isSuccess).toBe(true));
		expect(result.current.data).toEqual(volumes);

		expect(queryClient.getQueryData(volumesQueryKey("m-1"))).toEqual(volumes);
	});

	it("surfaces request errors without retrying", async () => {
		server.use(
			http.get("*/api/machines/m-1/volumes", () =>
				HttpResponse.json(
					{
						requestId: "r-1",
						error: { code: "INTERNAL_ERROR", message: "ssh failed" },
					},
					{ status: 500 },
				),
			),
		);

		const { result } = renderHookWithQuery(() => useVolumes("m-1"));
		await waitFor(() => expect(result.current.isError).toBe(true));
		expect(result.current.data).toBeUndefined();
	});
});
