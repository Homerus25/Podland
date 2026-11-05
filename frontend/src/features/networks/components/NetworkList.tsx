import { Button, Td, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { CreateNetworkDialog } from "@features/networks/components/CreateNetworkDialog";
import {
	useCreateNetwork,
	useNetworks,
	usePruneNetworks,
} from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { PruneReportDialog } from "@shared/components/PruneReportDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The prune adapter over PruneReportDialog, the twin of the containers
// one: the module owns the plumbing, this adapter hands over the noun
// and the two entry accessors that make a network entry a report row.
// Podman's network prune report keys entries by name and carries no
// size, so the reclaimed column renders an em dash like the pods one.
function PruneNetworksModal({
	onClose,
	pruneNetworks,
}: {
	onClose: () => void;
	pruneNetworks: ReturnType<typeof usePruneNetworks>;
}) {
	return (
		<PruneReportDialog
			mutation={pruneNetworks}
			resource="network"
			entryKey={(entry) => entry.name}
			renderLabel={(entry) => entry.name}
			onClose={onClose}
		/>
	);
}

// The Networks tab body. Create, connect, and prune are write-gated
// backend routes (issue 255), so the adapter takes the host and gates
// on manage-networks: below write level ResourceList swaps the toolbar
// for the canonical read hint and the rows drop their connect button. The
// toolbar/triad/table plumbing and the derived testids are ResourceList's;
// this adapter owns only the columns, the row, and the create dialog. The
// row's connect button opens the one connect dialog (issue 274) with the
// network fixed — the dialog owns the ritual: its options and the mutation.
// Clicking the row navigates to the network's detail page (spec 301); the
// connect button stops propagation so it never also fires the row action.
export function NetworkList({ host }: { host: Host }) {
	const hostId = host.id;
	const navigate = useNavigate();
	const { data: networks, isPending, error } = useNetworks(hostId);
	const createNetwork = useCreateNetwork(hostId);
	const pruneNetworks = usePruneNetworks(hostId);
	const canManage = can(host, "manage-networks");
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isPruneOpen, setIsPruneOpen] = useState(false);
	const [pendingConnect, setPendingConnect] = useState<NetworkSummary | null>(
		null,
	);

	return (
		<>
			<ResourceList
				resource="network"
				isPending={isPending}
				error={error}
				items={networks}
				columns={[
					{ label: "Name", isName: true, sortBy: (n) => n.name },
					{ label: "Driver", sortBy: (n) => n.driver },
					{ label: "Subnets", sortBy: (n) => n.subnets.join(", ") },
					{ label: "Created", sortBy: (n) => n.createdAt },
					...(canManage ? [{}] : []),
				]}
				renderRow={(network: NetworkSummary) => (
					<Tr
						key={network.name}
						data-testid="network-row"
						cursor="pointer"
						onClick={() =>
							navigate(detailPath(hostId, "networks", network.name))
						}
					>
						<Td fontFamily="mono">{network.name}</Td>
						<Td>{network.driver}</Td>
						<Td fontFamily="mono">{network.subnets.join(", ")}</Td>
						<Td>{formatRelativeDate(network.createdAt)}</Td>
						{canManage ? (
							<Td>
								<Button
									size="xs"
									variant="outline"
									aria-label={`Connect container ${network.name}`}
									onClick={(event) => {
										event.stopPropagation();
										setPendingConnect(network);
									}}
								>
									Connect container
								</Button>
							</Td>
						) : null}
					</Tr>
				)}
				toolbar={
					<>
						<Button
							size="sm"
							colorScheme="blue"
							onClick={() => setIsCreateOpen(true)}
							data-testid="create-network-button"
						>
							Create network
						</Button>
						<Button
							size="sm"
							colorScheme="red"
							variant="outline"
							onClick={() => setIsPruneOpen(true)}
							data-testid="prune-networks-button"
						>
							Prune
						</Button>
					</>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<CreateNetworkDialog
					hostId={hostId}
					mutation={createNetwork}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
			{isPruneOpen ? (
				<PruneNetworksModal
					onClose={() => setIsPruneOpen(false)}
					pruneNetworks={pruneNetworks}
				/>
			) : null}
			{pendingConnect ? (
				<ConnectDialog
					hostId={hostId}
					network={pendingConnect}
					onClose={() => setPendingConnect(null)}
				/>
			) : null}
		</>
	);
}
