import { Td, Tr } from "@chakra-ui/react";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { ResourceList } from "@shared/components/ResourceList";
import { formatRelativeDate } from "@shared/utils/format";

// The Networks tab body. The surface is read-only (issue 244: the list
// endpoint is the only one), so the adapter takes only the machine id and
// passes no toolbar and no canManage — ungated, no permission hint ever.
// The toolbar/triad/table machinery is ResourceList's; this adapter owns
// only the columns and the row.
export function NetworkList({ machineId }: { machineId: string }) {
	const { data: networks, isPending, error } = useNetworks(machineId);

	return (
		<ResourceList
			resource="network"
			isPending={isPending}
			error={error}
			items={networks}
			columns={[
				{ label: "Name" },
				{ label: "Driver" },
				{ label: "Subnets" },
				{ label: "Created" },
			]}
			renderRow={(network: NetworkSummary) => (
				<Tr key={network.name} data-testid="network-row">
					<Td fontFamily="mono">{network.name}</Td>
					<Td>{network.driver}</Td>
					<Td fontFamily="mono">{network.subnets.join(", ")}</Td>
					<Td>{formatRelativeDate(network.createdAt)}</Td>
				</Tr>
			)}
		/>
	);
}
