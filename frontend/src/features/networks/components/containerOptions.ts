import type { ContainerSummary } from "@features/containers/services/containersApi";

// The dropdowns offer running containers only — podman rejects attaching a
// stopped one (spec #251, decision 7).
export function runningContainers(
	containers: ContainerSummary[] | undefined,
): ContainerSummary[] {
	return (containers ?? []).filter(
		(container) => container.state === "running",
	);
}
