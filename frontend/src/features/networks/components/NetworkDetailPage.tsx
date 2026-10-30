import { Link, Table, Tbody, Text } from "@chakra-ui/react";
import {
	useNetwork,
	useNetworkInspect,
} from "@features/networks/hooks/useNetworks";
import type { NetworkDetail } from "@features/networks/services/networksApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailList,
	DetailRow,
	isNotFound,
} from "@shared/components/detailKit";
import { detailPath } from "@shared/routes/detailRoute";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { Link as ReactRouterLink, useParams } from "react-router-dom";

// The overview rows: the network's key attributes as label/value rows.
// Each attached container links to its container detail page (spec 301
// cross-links); a network without containers shows None.
function NetworkOverview({
	machineId,
	network,
}: {
	machineId: string;
	network: NetworkDetail;
}) {
	return (
		<Table size="sm">
			<Tbody>
				<DetailRow label="Name">
					<Text fontFamily="mono">{network.name}</Text>
				</DetailRow>
				<DetailRow label="Id">
					<Text fontFamily="mono" wordBreak="break-all">
						{network.id}
					</Text>
				</DetailRow>
				<DetailRow label="Driver">{network.driver}</DetailRow>
				<DetailRow label="Containers">
					<DetailList
						items={network.containers}
						getKey={(container) => container.id}
						testId="network-detail-containers"
						renderItem={(container) => (
							<Link
								as={ReactRouterLink}
								to={detailPath(machineId, "containers", container.id)}
								color="blue.500"
								data-testid={`network-container-link-${container.id}`}
							>
								{container.name}
							</Link>
						)}
					/>
				</DetailRow>
				<DetailRow label="Created">
					{formatRelativeDate(network.createdAt)}
				</DetailRow>
			</Tbody>
		</Table>
	);
}

// The network detail page (spec 301, ticket #309): an adapter over the
// shared DetailPage layout. It owns the overview query (the freshness
// factory's one-shot flavor, so every page visit refetches) and the lazy
// inspect query (enabled on first reveal), and hands the layout its
// structural slices; the layout owns the shell. The back button returns
// to the Networks tab.
export function NetworkDetailPage() {
	const { machineId, networkName } = useParams<{
		machineId: string;
		networkName: string;
	}>();
	const [revealed, setRevealed] = useState(false);
	const {
		data: network,
		isPending,
		error,
	} = useNetwork(machineId!, networkName!);
	const inspectQuery = useNetworkInspect(machineId!, networkName!, {
		enabled: revealed,
	});

	return (
		<DetailPage
			title={networkName!}
			backTo="networks"
			isPending={isPending}
			error={error}
			notFound={isNotFound(error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{network ? (
				<NetworkOverview machineId={machineId!} network={network} />
			) : null}
		</DetailPage>
	);
}
