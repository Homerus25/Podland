import { Button, Text, VStack } from "@chakra-ui/react";
import { MachineForm } from "@features/machines/components/MachineForm";
import { ShareMachineSection } from "@features/machines/components/ShareMachineSection";
import {
	useDeleteMachine,
	useUpdateMachine,
} from "@features/machines/hooks/useMachines";
import { can } from "@features/machines/permissions";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { PermissionHint } from "@shared/components/PermissionHint";
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
	// A cancelled save still runs to completion on the server; this flag
	// suppresses that late settle so neither the saved banner nor the error
	// envelope resurrects over the reverted fields. The next save clears it.
	const [cancelledSave, setCancelledSave] = useState(false);

	// The tab gates once, here. Every act it renders — metadata edit,
	// sharing, deletion — is an owner capability on the backend, so a
	// non-owner (read, write, or admin grantee) gets exactly one truthful
	// hint instead of a form whose every save is a 403.
	if (!can(machine, "edit-machine")) {
		return (
			<PermissionHint scope="settings" requirement="owner" phrase="edit" />
		);
	}
	// Invariant: only the machine's owner renders below — the share and
	// delete sections need no further gating.
	return (
		<VStack spacing={4} align="stretch">
			{updateMachine.isSuccess && !cancelledSave && (
				<Text color="green.500" data-testid="machine-saved">
					Changes saved.
				</Text>
			)}
			<MachineForm
				mode="edit"
				machine={machine}
				onSubmit={(input) => {
					setCancelledSave(false);
					return mutationToPromise(updateMachine.mutate, input);
				}}
				// Cancel resets the mutation: isSuccess drops (the saved banner
				// disappears) and the error envelope clears with the fields
				// reverted by the form itself. While a save is in flight the
				// suppression flag also latches, so its late settle stays hidden.
				onCancel={() => {
					if (updateMachine.isPending) setCancelledSave(true);
					updateMachine.reset();
				}}
				submitError={cancelledSave ? null : updateMachine.error}
				pending={updateMachine.isPending}
			/>
			<Text fontWeight="bold">Sharing</Text>
			<ShareMachineSection machineId={machine.id} ownerId={machine.ownerId} />
			<DeleteMachineSection machine={machine} />
		</VStack>
	);
}
