import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import { useContainerInspect } from "@features/containers/hooks/useContainers";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { InspectDialog } from "@shared/components/InspectDialog";

// Thin adapter between the containers feature and the shared inspect
// dialog: it owns the query (fired on mount, dead on unmount, gcTime 0 so
// every open refetches) and hands the dialog its structural slice; the
// dialog owns the whole render. The title is the container's display name.
export function InspectContainerDialog({
	machineId,
	container,
	onClose,
}: {
	machineId: string;
	container: ContainerSummary;
	onClose: () => void;
}) {
	const query = useContainerInspect(machineId, container.id);
	return (
		<InspectDialog
			title={containerDisplayName(container)}
			query={query}
			onClose={onClose}
		/>
	);
}
