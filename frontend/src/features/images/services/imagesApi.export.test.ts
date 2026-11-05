import { imageExportUrl } from "@features/images/services/imagesApi";
import { describe, expect, it } from "vitest";

// Service-level test for the image export URL builder (issue 401): the
// fetch-based download helper consumes the full-URL form, and the image
// ID (a sha256: digest) must be percent-encoded as a path segment.
const imageId =
	"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

describe("imageExportUrl", () => {
	it("builds the full export URL with the image ID encoded", () => {
		expect(imageExportUrl("m-1", imageId)).toBe(
			`/api/hosts/m-1/images/${encodeURIComponent(imageId)}/export`,
		);
	});
});
