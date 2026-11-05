import { Button, Heading, HStack, SimpleGrid, VStack } from "@chakra-ui/react";
import { HostCard } from "@features/hosts/components/HostCard";
import { HostForm } from "@features/hosts/components/HostForm";
import { useCreateHost, useHosts } from "@features/hosts/hooks/useHosts";
import { ResourceStates } from "@shared/components/ResourceStates";
import { useState } from "react";

export function HostsPage() {
	const { data: hosts, isPending, error } = useHosts();
	const createHost = useCreateHost();
	const [isAddOpen, setIsAddOpen] = useState(false);

	return (
		<VStack spacing={4} align="stretch">
			<HStack justify="space-between">
				<Heading size="lg">Hosts</Heading>
				<Button
					colorScheme="blue"
					onClick={() => setIsAddOpen(true)}
					data-testid="add-host-button"
				>
					Add Host
				</Button>
			</HStack>
			<ResourceStates
				resource="host"
				isPending={isPending}
				error={error}
				items={hosts}
				emptyMessage="No hosts yet. Register a host to start managing its Podman containers — or ask an owner to share one with you. Once you have access, it will appear here."
			>
				<SimpleGrid columns={[1, null, 2, 3]} spacing={4}>
					{hosts?.map((host) => (
						<HostCard key={host.id} host={host} />
					))}
				</SimpleGrid>
			</ResourceStates>
			{isAddOpen ? (
				<HostForm
					mode="create"
					mutation={createHost}
					onClose={() => setIsAddOpen(false)}
				/>
			) : null}
		</VStack>
	);
}
