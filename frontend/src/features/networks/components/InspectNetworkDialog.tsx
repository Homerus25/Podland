import { useNetworkInspect } from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { InspectDialog } from "@shared/components/InspectDialog";

// Thin adapter between the networks feature and the shared inspect
// dialog: it owns the query (fired on mount, dead on unmount, gcTime 0 so
// every open refetches) and hands the dialog its structural slice; the
// dialog owns the whole render. The title is the network's name.
export function InspectNetworkDialog({
	machineId,
	network,
	onClose,
}: {
	machineId: string;
	network: NetworkSummary;
	onClose: () => void;
}) {
	const query = useNetworkInspect(machineId, network.name);
	return <InspectDialog title={network.name} query={query} onClose={onClose} />;
}
