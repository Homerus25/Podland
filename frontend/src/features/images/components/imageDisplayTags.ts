import type { ImageSummary } from "@features/images/services/imagesApi";

// Podman reports an image's tags in repoTags, and an untagged image carries
// the placeholder "<none>:<none>", which reads as noise — the display falls
// back to a bare "<none>". The images feature owns ImageSummary, so the
// display-tag rule lives here — every surface that renders an image's tags
// imports it.
function imageTags(image: ImageSummary): string[] {
	const tags = (image.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
	return tags.length > 0 ? tags : ["<none>"];
}

export function displayTags(image: ImageSummary): string {
	return imageTags(image).join(", ");
}
