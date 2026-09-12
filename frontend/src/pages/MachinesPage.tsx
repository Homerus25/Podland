import {
	Button,
	Heading,
	HStack,
	SimpleGrid,
	Text,
	VStack,
} from "@chakra-ui/react";
import { MachineCard } from "@features/machines/components/MachineCard";
import { MachineForm } from "@features/machines/components/MachineForm";
import {
	useCreateMachine,
	useMachines,
} from "@features/machines/hooks/useMachines";
import type { CreateMachineInput } from "@features/machines/services/machinesApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { MutationDialog } from "@shared/components/MutationDialog";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";

export function MachinesPage() {
	const { data: machines, isPending, error } = useMachines();
	const createMachine = useCreateMachine();
	const [isAddOpen, setIsAddOpen] = useState(false);

	return (
		<VStack spacing={4} align="stretch">
			<HStack justify="space-between">
				<Heading size="lg">Machines</Heading>
				<Button
					colorScheme="blue"
					onClick={() => setIsAddOpen(true)}
					data-testid="add-machine-button"
				>
					Add Machine
				</Button>
			</HStack>
			{isPending && (
				<Text data-testid="machines-loading">Loading machines…</Text>
			)}
			{error && <ErrorDisplay error={getApiError(error)} />}
			{machines &&
				(machines.length === 0 ? (
					<Text data-testid="machines-empty">
						No machines yet. Register a machine to start managing its Podman
						containers — or ask an owner to share one with you. Once you have
						access, it will appear here.
					</Text>
				) : (
					<SimpleGrid columns={[1, null, 2, 3]} spacing={4}>
						{machines.map((machine) => (
							<MachineCard key={machine.id} machine={machine} />
						))}
					</SimpleGrid>
				))}
			{isAddOpen ? (
				<MutationDialog
					mutation={createMachine}
					title="Add Machine"
					onClose={() => setIsAddOpen(false)}
					body={
						<MachineForm
							onSubmit={(input) =>
								createMachine.mutate(input as CreateMachineInput)
							}
							isSubmitting={createMachine.isPending}
						/>
					}
				/>
			) : null}
		</VStack>
	);
}
