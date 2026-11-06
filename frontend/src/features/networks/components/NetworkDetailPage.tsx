import { Link, Table, Tbody, Text } from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { NETWORK_IN_USE_REFUSAL } from "@features/networks/components/networkRemoval";
import {
	useDeleteNetwork,
	useNetwork,
	useNetworkInspect,
} from "@features/networks/hooks/useNetworks";
import type { NetworkDetail } from "@features/networks/services/networksApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailList,
	DetailRow,
	isNotFound,
	removalAction,
} from "@shared/components/detailKit";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { detailPath, hostTabPath } from "@shared/routes/detailRoute";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import {
	Link as ReactRouterLink,
	useNavigate,
	useParams,
} from "react-router-dom";

// The overview rows: the network's key attributes as label/value rows.
// Each attached container links to its container detail page (spec 301
// cross-links); a network without containers shows None.
function NetworkOverview({
	hostId,
	network,
}: {
	hostId: string;
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
								to={detailPath(hostId, "containers", container.id)}
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

// The Remove action's confirm (spec 461, ticket #465): the shared
// force-refusal preset, so the first confirm sends the plain delete and
// only the NETWORK_IN_USE refusal swaps in the in-use hint and the force
// button whose retry carries force=true. The adapter keeps the domain
// vocabulary only — the refusal words come from the networkRemoval module
// the list's modal reads too, and the success side effect (navigate back
// to the list tab) arrives through the preset's onSuccess seam.
function RemoveNetworkDialog({
	networkName,
	onClose,
	deleteNetwork,
	onSuccess,
}: {
	networkName: string;
	onClose: () => void;
	deleteNetwork: ReturnType<typeof useDeleteNetwork>;
	onSuccess: () => void;
}) {
	return (
		<ForceConfirmDialog
			mutation={deleteNetwork}
			title="Remove network"
			onClose={onClose}
			refusalCode={NETWORK_IN_USE_REFUSAL.code}
			hint={NETWORK_IN_USE_REFUSAL.hint}
			hintTestId={NETWORK_IN_USE_REFUSAL.hintTestId}
			confirmLabel="Remove network"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ networkName, force })}
			onSuccess={onSuccess}
			body={
				<Text>
					Remove network {networkName} from this host? This cannot be undone.
				</Text>
			}
		/>
	);
}

// The network detail page (spec 301, ticket #309): an adapter over the
// shared DetailPage layout. It owns the overview query (the freshness
// factory's one-shot flavor, so every page visit refetches) and the lazy
// inspect query (enabled on first reveal), and hands the layout its
// structural slices; the layout owns the shell. The back button returns
// to the Networks tab.
export function NetworkDetailPage() {
	const { hostId, networkName } = useParams<{
		hostId: string;
		networkName: string;
	}>();
	const navigate = useNavigate();
	const [revealed, setRevealed] = useState(false);
	// The remove confirm (spec 461, ticket #465): rendered conditionally, so
	// mount is open and unmount is close.
	const [removeOpen, setRemoveOpen] = useState(false);
	const { data: network, isPending, error } = useNetwork(hostId!, networkName!);
	const inspectQuery = useNetworkInspect(hostId!, networkName!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "manage-networks");
	const deleteNetwork = useDeleteNetwork(hostId!);
	const notFound = isNotFound(error);

	// The Remove action rides the detail-page capability gate (the kit owns
	// the cascade): allowed renders the button, denied the shared write hint,
	// pending neither, and a not-found overview hides it outright.
	const removeAction = removalAction({
		gate: gate,
		notFound,
		scope: "networks",
		resource: "network",
		testId: "remove-network-button",
		onClick: () => setRemoveOpen(true),
	});

	return (
		<>
			<DetailPage
				title={networkName!}
				backTo="networks"
				isPending={isPending}
				error={error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={removeAction}
			>
				{network ? (
					<NetworkOverview hostId={hostId!} network={network} />
				) : null}
			</DetailPage>
			{removeOpen ? (
				<RemoveNetworkDialog
					networkName={networkName!}
					onClose={() => setRemoveOpen(false)}
					deleteNetwork={deleteNetwork}
					// The removed network's page would 404 on the next refetch, so
					// the page leaves for the list tab; the mutation's own
					// invalidation refetches that list.
					onSuccess={() => navigate(hostTabPath(hostId!, "networks"))}
				/>
			) : null}
		</>
	);
}
