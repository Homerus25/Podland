import {
	Card,
	CardBody,
	CardHeader,
	Heading,
	HStack,
	Text,
} from "@chakra-ui/react";
import { PermissionBadge } from "@features/hosts/components/PermissionBadge";
import { useHostStatus } from "@features/hosts/hooks/useHosts";
import { HostStatusBadge } from "@shared/components/HostStatusBadge";
import type { Host } from "@shared/types/api";
import { useNavigate } from "react-router-dom";

export function HostCard({ host }: { host: Host }) {
	const navigate = useNavigate();
	const statusQuery = useHostStatus(host.id);

	// checking/unknown are client-side states; online/offline come from the
	// endpoint. Each card resolves on its own so slow or dead hosts never
	// block the list.
	const status = statusQuery.isPending
		? "checking"
		: statusQuery.isError
			? "unknown"
			: (statusQuery.data?.status ?? "checking");

	return (
		<Card
			as="button"
			type="button"
			data-testid="host-card"
			onClick={() => navigate(`/hosts/${host.id}`)}
			cursor="pointer"
			textAlign="left"
			_hover={{ shadow: "md" }}
		>
			<CardHeader>
				<HStack justify="space-between">
					<Heading size="md">{host.name}</Heading>
					<HStack spacing={2}>
						<HostStatusBadge
							status={status}
							checkedAt={statusQuery.data?.checkedAt}
						/>
						<PermissionBadge permission={host.permission} />
					</HStack>
				</HStack>
			</CardHeader>
			<CardBody>
				<Text color="gray.500">{host.description}</Text>
				<Text fontFamily="mono">
					{host.hostname}:{host.port}
				</Text>
			</CardBody>
		</Card>
	);
}
