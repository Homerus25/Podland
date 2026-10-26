import {
	getImageCopyPreview,
	imageCopyStreamUrl,
} from "@features/images/services/imagesApi";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { imageCopyPreviewFixture } from "@/test/factories";
import { server } from "@/test/server";

// Service-level tests for the image copy pair (issue 376): request shape
// and passthrough, like the other imagesApi suites. The wire contract is
// the backend's (issue 375, doku/description_images_api.md).
const imageId =
	"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

describe("getImageCopyPreview", () => {
	it("GETs the preview from the source image with the target as a query parameter", async () => {
		const captured: { path: string; query: URLSearchParams | null } = {
			path: "",
			query: null,
		};
		server.use(
			http.get(
				`*/api/machines/m-1/images/${encodeURIComponent(imageId)}/copy-preview`,
				({ request }) => {
					const url = new URL(request.url);
					captured.path = url.pathname;
					captured.query = url.searchParams;
					return HttpResponse.json(imageCopyPreviewFixture());
				},
			),
		);

		const preview = await getImageCopyPreview("m-1", imageId, "m-2");

		expect(captured.path).toBe(
			`/api/machines/m-1/images/${encodeURIComponent(imageId)}/copy-preview`,
		);
		expect(captured.query?.get("targetMachineId")).toBe("m-2");
		expect(preview).toEqual(imageCopyPreviewFixture());
	});

	it("passes the already-present fact through untouched", async () => {
		const facts = imageCopyPreviewFixture({ present: true });
		server.use(
			http.get(
				`*/api/machines/m-1/images/${encodeURIComponent(imageId)}/copy-preview`,
				() => HttpResponse.json(facts),
			),
		);

		const preview = await getImageCopyPreview("m-1", imageId, "m-2");

		expect(preview).toEqual(facts);
	});
});

describe("imageCopyStreamUrl", () => {
	it("builds the full copy-stream URL for the fetch transport", () => {
		expect(imageCopyStreamUrl("m-1", imageId)).toBe(
			`/api/machines/m-1/images/${encodeURIComponent(imageId)}/copy`,
		);
	});
});
