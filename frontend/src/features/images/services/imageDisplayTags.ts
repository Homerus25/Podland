import { shortImageId } from "@shared/utils/format";

// Podman reports an image's tags in repoTags, and an untagged image carries
// the placeholder "<none>:<none>", which reads as noise — the display falls
// back to a bare "<none>". The images feature owns the image DTOs, so the
// display-tag rule lives here — every surface that renders an image's tags
// imports it. The rule accepts any shape carrying repoTags: the list
// summary and the detail DTO both qualify. The module is pure DTO
// vocabulary, not a component, so it sits in the feature's services layer:
// hooks (the download hook's failure toast) import it as readily as
// components do — a hooks module never imports from components.
function imageTags(image: { repoTags: string[] | null }): string[] {
	const tags = (image.repoTags ?? []).filter((tag) => tag !== "<none>:<none>");
	return tags.length > 0 ? tags : ["<none>"];
}

export function displayTags(image: { repoTags: string[] | null }): string {
	return imageTags(image).join(", ");
}

// The naming composite every surface that must name one image unambiguously
// uses: the tags alone render "<none>" for an untagged image, so the short
// id disambiguates. The delete confirm, the download failure toast, and the
// bulk-remove report all read it from here.
export function imageDisplayName(image: {
	id: string;
	repoTags: string[] | null;
}): string {
	return `${displayTags(image)} (${shortImageId(image.id)})`;
}
