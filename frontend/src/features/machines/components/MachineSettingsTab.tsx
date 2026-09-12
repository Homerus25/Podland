import { Button, Text, VStack } from "@chakra-ui/react";
import { MachineForm } from "@features/machines/components/MachineForm";
import { ShareMachineSection } from "@features/machines/components/ShareMachineSection";
import {
	useDeleteMachine,
	useUpdateMachine,
} from "@features/machines/hooks/useMachines";
import {
	canDeleteMachine,
	canEditMachine,
	canShareMachine,
	effectivePermission,
} from "@features/machines/permissions";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import type { Machine } from "@shared/types/api";
import { mutationToPromise } from "@shared/utils/mutationPromise";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

function DeleteMachineSection({ machine }: { machine: Machine }) {
	const navigate = useNavigate();
	const deleteMachine = useDeleteMachine();
	const [isConfirmOpen, setIsConfirmOpen] = useState(false);

	// Deleting the machine destroys the page's subject, so success always
	// navigates back to the machines list. Failure stays put: the envelope
	// renders inside the dialog, which stays open for another attempt.
	return (
		<>
			<Button
				colorScheme="red"
				variant="outline"
				onClick={() => setIsConfirmOpen(true)}
				data-testid="delete-machine-button"
			>
				Delete machine
			</Button>
			{isConfirmOpen ? (
				<ConfirmDialog
					mutation={deleteMachine}
					title="Delete machine"
					confirmLabel="Delete machine"
					confirm={() => machine.id}
					onClose={() => setIsConfirmOpen(false)}
					onSuccess={() => navigate("/machines")}
					body={
						<Text>
							Delete “{machine.name}” ({machine.hostname}:{machine.port})? This
							removes the machine registration and all access grants. The remote
							host is not touched.
						</Text>
					}
				/>
			) : null}
		</>
	);
}

export function MachineSettingsTab({ machine }: { machine: Machine }) {
	const updateMachine = useUpdateMachine(machine.id);
	const permission = effectivePermission(machine);

	if (!canEditMachine(permission)) {
		return (
			<Text
				color="gray.500"
				data-testid="permission-hint"
				data-permission="read"
			>
				You have read-only access to this machine. Ask an owner or admin for
				write access to edit it.
			</Text>
		);
	}
	return (
		<VStack spacing={4} align="stretch">
			{updateMachine.isSuccess && (
				<Text color="green.500" data-testid="machine-saved">
					Changes saved.
				</Text>
			)}
			<MachineForm
				mode="edit"
				machine={machine}
				onSubmit={(input) => mutationToPromise(updateMachine.mutate, input)}
				submitError={updateMachine.error}
				isSubmitting={updateMachine.isPending}
			/>
			{canShareMachine(permission) ? (
				<>
					<Text fontWeight="bold">Sharing</Text>
					<ShareMachineSection machineId={machine.id} />
				</>
			) : null}
			{canDeleteMachine(permission) ? (
				<DeleteMachineSection machine={machine} />
			) : (
				<Text
					color="gray.500"
					data-testid="permission-hint"
					data-permission="write"
				>
					Only the owner or an admin can share or delete this machine.
				</Text>
			)}
		</VStack>
	);
}
