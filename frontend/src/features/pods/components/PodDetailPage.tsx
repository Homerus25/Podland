import { HStack, Table, Tbody, Text } from "@chakra-ui/react";
import { usePod, usePodInspect } from "@features/pods/hooks/usePods";
import { DetailPage } from "@shared/components/DetailPage";
import { DetailRow, isNotFound } from "@shared/components/detailKit";
import { detailPath } from "@shared/routes/detailRoute";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";

// The pod detail page (spec 352): the shared layout's shell with the
// pod's detail DTO as overview rows and cross-links to the containers in
// the pod. The overview is the usePod detail query (gcTime: 0, refetched
// on every visit); the raw inspect report fetches lazily on the first
// "Show inspect JSON" click and never on page mount.
export function PodDetailPage() {
	const { machineId, podId } = useParams<{
		machineId: string;
		podId: string;
	}>();
	const [revealed, setRevealed] = useState(false);
	const pod = usePod(machineId!, podId!);
	const inspectQuery = usePodInspect(machineId!, podId!, {
		enabled: revealed,
	});

	return (
		<DetailPage
			title={pod.data?.name ?? podId!}
			backTo="pods"
			isPending={pod.isPending}
			error={pod.error}
			notFound={isNotFound(pod.error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{pod.data ? (
				<Table size="sm">
					<Tbody>
						<DetailRow label="ID">
							<Text fontFamily="mono" data-testid="pod-detail-id">
								{pod.data.id}
							</Text>
						</DetailRow>
						<DetailRow label="Name">
							<Text fontFamily="mono" data-testid="pod-detail-name">
								{pod.data.name}
							</Text>
						</DetailRow>
						<DetailRow label="State">
							<Text data-testid="pod-detail-state">{pod.data.state}</Text>
						</DetailRow>
						<DetailRow label="Created">
							<Text data-testid="pod-detail-created">
								{formatRelativeDate(pod.data.createdAt)}
							</Text>
						</DetailRow>
						<DetailRow label="Restart policy">
							<Text data-testid="pod-detail-restart-policy">
								{pod.data.restartPolicy || "—"}
							</Text>
						</DetailRow>
						{pod.data.labels && Object.keys(pod.data.labels).length > 0 ? (
							<DetailRow label="Labels">
								<HStack data-testid="pod-detail-labels">
									{Object.entries(pod.data.labels).map(([key, value]) => (
										<Text key={key} fontFamily="mono">
											{key}={value}
										</Text>
									))}
								</HStack>
							</DetailRow>
						) : null}
						{pod.data.mounts && pod.data.mounts.length > 0 ? (
							<DetailRow label="Mounts">
								<HStack data-testid="pod-detail-mounts">
									{pod.data.mounts.map((mount) => (
										<Text key={mount} fontFamily="mono">
											{mount}
										</Text>
									))}
								</HStack>
							</DetailRow>
						) : null}
						{pod.data.networks && pod.data.networks.length > 0 ? (
							<DetailRow label="Networks">
								<HStack data-testid="pod-detail-networks">
									{pod.data.networks.map((network) => (
										<Text key={network} fontFamily="mono">
											{network}
										</Text>
									))}
								</HStack>
							</DetailRow>
						) : null}
						<DetailRow label="Containers">
							<HStack data-testid="pod-detail-containers">
								{pod.data.containers.map((container) => (
									// Cross-link to the container's detail page (spec 301):
									// the pod's containers are first-class resources.
									<Link
										key={container.id}
										to={detailPath(machineId!, "containers", container.id)}
										color="blue.500"
										data-testid="pod-detail-container-link"
									>
										{container.name} ({container.state})
									</Link>
								))}
								{pod.data.containers.length === 0 ? (
									<Text color="gray.500">No containers in this pod.</Text>
								) : null}
							</HStack>
						</DetailRow>
					</Tbody>
				</Table>
			) : null}
		</DetailPage>
	);
}
