// Podman reports an image's tags in repoTags, and an untagged image carries
// the placeholder "<none>:<none>", which reads as noise — the display falls
// back to a bare "<none>". The images feature owns the image DTOs, so the
// display-tag rule lives here — every surface that renders an image's tags
// imports it. The rule accepts any shape carrying repoTags: the list
// summary and the detail DTO both qualify.
function imageTags(image: { repoTags: string[] | null }): string[] {
	const tags = (image.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
	return tags.length > 0 ? tags : ["<none>"];
}

export function displayTags(image: { repoTags: string[] | null }): string {
	return imageTags(image).join(", ");
}
