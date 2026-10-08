import type { ContainerSummary } from "@features/containers/services/containersApi";
import { shortImageId } from "@shared/utils/format";

// Podman reports container names with a leading slash; display them bare
// and fall back to the short id when a container has no name at all. The
// containers feature owns ContainerSummary, so the display-name rule lives
// here — every surface that renders a container's name imports it.
export function containerDisplayName(container: ContainerSummary): string {
	const name = container.names?.[0]?.replace(/^\//, "");
	return name ? name : shortImageId(container.id);
}
