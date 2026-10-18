import {
	Link as ChakraLink,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Tr,
} from "@chakra-ui/react";
import {
	useContainer,
	useContainerInspect,
} from "@features/containers/hooks/useContainers";
import type { ContainerDetail } from "@features/containers/services/containersApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailList,
	DetailRow,
	isNotFound,
} from "@shared/components/detailKit";
import { StateBadge } from "@shared/components/StateBadge";
import { detailPath } from "@shared/routes/detailRoute";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { Link as ReactRouterLink, useParams } from "react-router-dom";

// The container's overview: one label/value row per attribute the spec
// lists. Cross-links (image, secrets) point at the related resources'
// detail pages.
function ContainerOverview({
	machineId,
	container,
}: {
	machineId: string;
	container: ContainerDetail;
}) {
	const environment = Object.entries(container.environment ?? {});
	return (
		<TableContainer data-testid="container-detail-overview">
			<Table size="sm">
				<Tbody>
					<DetailRow label="Name">
						<Text fontFamily="mono" data-testid="container-detail-name">
							{container.name}
						</Text>
					</DetailRow>
					<DetailRow label="Id">
						<Text fontFamily="mono" data-testid="container-detail-id">
							{container.id}
						</Text>
					</DetailRow>
					<DetailRow label="State">
						<StateBadge state={container.state} />
					</DetailRow>
					<DetailRow label="ImageName">
						{container.imageId ? (
							<ChakraLink
								as={ReactRouterLink}
								to={detailPath(machineId, "images", container.imageId)}
								color="blue.500"
								data-testid="container-image-link"
							>
								{container.image}
							</ChakraLink>
						) : (
							<Text>{container.image}</Text>
						)}
					</DetailRow>
					<DetailRow label="Volumes">
						<DetailList
							items={container.volumes}
							getKey={(volume) => volume.name}
							testId="container-detail-volumes"
							renderItem={(volume) => (
								<>
									{volume.name}: {volume.source} → {volume.destination} (
									{volume.rw ? "rw" : "ro"})
								</>
							)}
						/>
					</DetailRow>
					<DetailRow label="Mounts">
						<DetailList
							items={container.mounts}
							getKey={(mount) => mount}
							testId="container-detail-mounts"
							mono
							renderItem={(mount) => mount}
						/>
					</DetailRow>
					<DetailRow label="Networks">
						<DetailList
							items={container.networks}
							getKey={(network) => network}
							testId="container-detail-networks"
							renderItem={(network) => network}
						/>
					</DetailRow>
					<DetailRow label="Exposed Ports">
						<DetailList
							items={container.exposedPorts}
							getKey={(port) => `${port.containerPort}/${port.protocol}`}
							testId="container-detail-exposed-ports"
							renderItem={(port) => `${port.containerPort}/${port.protocol}`}
						/>
					</DetailRow>
					<DetailRow label="Entrypoint">
						<Text data-testid="container-detail-entrypoint">
							{container.entrypoint && container.entrypoint.length > 0
								? container.entrypoint.join(" ")
								: "None"}
						</Text>
					</DetailRow>
					<DetailRow label="Environment Variables">
						{environment.length === 0 ? (
							<Text color="gray.500">None</Text>
						) : (
							<TableContainer data-testid="container-detail-environment">
								<Table size="sm">
									<Tbody>
										{environment.map(([key, value]) => (
											<Tr key={key}>
												<Td fontFamily="mono" width="40%">
													{key}
												</Td>
												<Td fontFamily="mono">{value}</Td>
											</Tr>
										))}
									</Tbody>
								</Table>
							</TableContainer>
						)}
					</DetailRow>
					<DetailRow label="Secrets">
						<DetailList
							items={container.secrets}
							getKey={(secret) => secret.name}
							testId="container-detail-secrets"
							renderItem={(secret) => (
								<ChakraLink
									as={ReactRouterLink}
									to={detailPath(machineId, "secrets", secret.name)}
									color="blue.500"
									data-testid={`container-secret-link-${secret.name}`}
								>
									{secret.name}
								</ChakraLink>
							)}
						/>
					</DetailRow>
					<DetailRow label="Created">
						<Text data-testid="container-detail-created">
							{formatRelativeDate(container.createdAt)}
						</Text>
					</DetailRow>
					<DetailRow label="Cmd">
						<Text data-testid="container-detail-command">
							{container.command && container.command.length > 0
								? container.command.join(" ")
								: "None"}
						</Text>
					</DetailRow>
				</Tbody>
			</Table>
		</TableContainer>
	);
}

// The container detail page (spec 301, ticket #306): the shared layout's
// shell with this feature's overview rows. The overview query refetches on
// every visit (gcTime: 0); the raw inspect report fetches lazily on the
// first "Show inspect JSON" click, never on page mount.
export function ContainerDetailPage() {
	const { machineId, containerId } = useParams<{
		machineId: string;
		containerId: string;
	}>();
	const query = useContainer(machineId!, containerId!);
	const [revealed, setRevealed] = useState(false);
	const inspectQuery = useContainerInspect(machineId!, containerId!, {
		enabled: revealed,
	});
	const container = query.data;

	return (
		<DetailPage
			title={container?.name ?? containerId!}
			backTo="containers"
			isPending={query.isPending}
			error={query.error}
			notFound={isNotFound(query.error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{container ? (
				<ContainerOverview machineId={machineId!} container={container} />
			) : null}
		</DetailPage>
	);
}
