import { Button, Td, Tr } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import { ConnectDialog } from "@features/networks/components/ConnectDialog";
import { CreateNetworkDialog } from "@features/networks/components/CreateNetworkDialog";
import {
	useCreateNetwork,
	useNetworks,
} from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { ResourceList } from "@shared/components/ResourceList";
import type { Machine } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";

// The Networks tab body. Create and connect are write-gated backend routes
// (issue 255), so the adapter takes the machine and gates on
// manage-networks: below write level ResourceList swaps the toolbar for the
// canonical read hint and the rows drop their connect button. The
// toolbar/triad/table machinery and the derived testids are ResourceList's;
// this adapter owns only the columns, the row, and the create dialog. The
// row's connect button opens the one connect dialog (issue 274) with the
// network fixed — the dialog owns the ritual: its options and the mutation.
export function NetworkList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: networks, isPending, error } = useNetworks(machineId);
	const createNetwork = useCreateNetwork(machineId);
	const canManage = can(machine, "manage-networks");
	const [isCreateOpen, setIsCreateOpen] = useState(false);
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
					{ label: "Name", sortBy: (n) => n.name },
					{ label: "Driver", sortBy: (n) => n.driver },
					{ label: "Subnets", sortBy: (n) => n.subnets.join(", ") },
					{ label: "Created", sortBy: (n) => n.createdAt },
					...(canManage ? [{}] : []),
				]}
				renderRow={(network: NetworkSummary) => (
					<Tr key={network.name} data-testid="network-row">
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
									onClick={() => setPendingConnect(network)}
								>
									Connect container
								</Button>
							</Td>
						) : null}
					</Tr>
				)}
				toolbar={
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => setIsCreateOpen(true)}
						data-testid="create-network-button"
					>
						Create network
					</Button>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<CreateNetworkDialog
					machineId={machineId}
					mutation={createNetwork}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
			{pendingConnect ? (
				<ConnectDialog
					machineId={machineId}
					network={pendingConnect}
					onClose={() => setPendingConnect(null)}
				/>
			) : null}
		</>
	);
}
