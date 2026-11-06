import { shortImageId } from "@shared/utils/format";

// The shape the display-name rule reads: the container's id and whichever
// name field its DTO carries. The list summary names a container through
// `names` (podman's array, leading slash and all); the detail DTO carries a
// single `name`. Accepting both keeps one rule for every surface instead of
// a projection at each call site.
export type ContainerNamed = {
	id: string;
	names?: string[];
	name?: string;
};

// Podman reports container names with a leading slash; display them bare
// and fall back to the short id when a container has no name at all. The
// containers feature owns the container DTOs, so the display-name rule lives
// here — every surface that renders a container's name imports it.
export function containerDisplayName(container: ContainerNamed): string {
	const name = container.names?.[0]?.replace(/^\//, "") ?? container.name;
	return name ? name : shortImageId(container.id);
}
