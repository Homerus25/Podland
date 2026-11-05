import {
	getVolumeMovePreview,
	volumeMoveStreamUrl,
} from "@features/volumes/services/volumesApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { volumeMovePreviewFixture } from "@/test/factories";
import { server } from "@/test/server";

// Service-level tests for the volume copy/move pair (issue 238): request
// shape and passthrough, like the other volumesApi suites.
describe("getVolumeMovePreview", () => {
	it("GETs the preview from the source volume with the target as a query parameter", async () => {
		const captured: { path: string; query: URLSearchParams | null } = {
			path: "",
			query: null,
		};
		server.use(
			http.get(
				"*/api/hosts/m-1/volumes/app-data/move-preview",
				({ request }) => {
					const url = new URL(request.url);
					captured.path = url.pathname;
					captured.query = url.searchParams;
					return HttpResponse.json(volumeMovePreviewFixture());
				},
			),
		);

		const preview = await getVolumeMovePreview("m-1", "app-data", "m-2");

		expect(captured.path).toBe("/api/hosts/m-1/volumes/app-data/move-preview");
		expect(captured.query?.get("targetHostId")).toBe("m-2");
		expect(preview).toEqual(volumeMovePreviewFixture());
	});

	it("passes the preview facts through untouched", async () => {
		const facts = volumeMovePreviewFixture({
			collision: true,
			inUse: true,
			referencingQuadlets: [
				{ name: "web.container" },
				{ name: "db.container" },
			],
		});
		server.use(
			http.get("*/api/hosts/m-1/volumes/app-data/move-preview", () =>
				HttpResponse.json(facts),
			),
		);

		const preview = await getVolumeMovePreview("m-1", "app-data", "m-2");

		expect(preview).toEqual(facts);
	});
});

describe("volumeMoveStreamUrl", () => {
	it("builds the full move-stream URL for the fetch transport", () => {
		expect(volumeMoveStreamUrl("m-1", "app-data")).toBe(
			"/api/hosts/m-1/volumes/app-data/move",
		);
	});
});
