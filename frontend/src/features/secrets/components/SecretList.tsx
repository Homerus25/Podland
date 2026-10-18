import { Button, HStack, Td, Text, Tr } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import { CreateSecretDialog } from "@features/secrets/components/CreateSecretDialog";
import { MoveSecretDialog } from "@features/secrets/components/MoveSecretDialog";
import { SecretDetailModal } from "@features/secrets/components/SecretDetailModal";
import {
	useCreateSecret,
	useDeleteSecret,
	useSecrets,
} from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { ResourceList } from "@shared/components/ResourceList";
import type { Machine } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";

// Each row owns its delete mutation instance, so pending state and errors
// stay scoped to this secret's modal and other rows stay usable. The shell,
// reset lifecycle, danger footer, close-on-success, and error region are
// ConfirmDialog's; the failure envelopes (409 SECRET_IN_USE, 404) surface
// through the default error region as plain envelopes, with no code-specific
// hint. There are no delete options in v1: the backend exposes no force
// parameter.
function SecretRow({
	machine,
	secret,
	canDelete,
	canMove,
}: {
	machine: Machine;
	secret: SecretSummary;
	canDelete: boolean;
	canMove: boolean;
}) {
	const machineId = machine.id;
	const deleteSecret = useDeleteSecret(machineId);
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	const [isDetailOpen, setIsDetailOpen] = useState(false);
	const [isMoveOpen, setIsMoveOpen] = useState(false);

	return (
		<>
			<Tr data-testid="secret-row">
				<Td fontFamily="mono">
					<Button
						size="xs"
						variant="link"
						onClick={() => setIsDetailOpen(true)}
						data-testid="secret-detail-button"
					>
						{secret.name}
					</Button>
				</Td>
				<Td>{secret.driver}</Td>
				<Td>{formatRelativeDate(secret.createdAt)}</Td>
				{canDelete ? (
					<Td>
						<HStack spacing={2}>
							{canMove ? (
								<Button
									size="xs"
									variant="outline"
									aria-label={`Move or copy ${secret.name}`}
									onClick={() => setIsMoveOpen(true)}
									data-testid="move-secret-button"
								>
									Move/Copy
								</Button>
							) : null}
							<Button
								size="xs"
								colorScheme="red"
								variant="outline"
								aria-label={`Delete ${secret.name}`}
								onClick={() => setIsDeleteOpen(true)}
								data-testid="delete-secret-button"
							>
								Delete
							</Button>
						</HStack>
					</Td>
				) : null}
			</Tr>
			{isDetailOpen ? (
				<SecretDetailModal
					machine={machine}
					secret={secret}
					onClose={() => setIsDetailOpen(false)}
				/>
			) : null}
			{isMoveOpen ? (
				<MoveSecretDialog
					machine={machine}
					secret={secret}
					onClose={() => setIsMoveOpen(false)}
				/>
			) : null}
			{isDeleteOpen ? (
				<ConfirmDialog
					mutation={deleteSecret}
					title="Delete secret"
					confirmLabel="Delete secret"
					confirm={() => secret.name}
					onClose={() => setIsDeleteOpen(false)}
					body={<Text>Delete “{secret.name}”? This cannot be undone.</Text>}
				/>
			) : null}
		</>
	);
}

// The Secrets tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on the machine's per-caller
// facts. The toolbar/triad/table machinery and the hint swap are
// ResourceList's; this adapter owns only the columns, the row, and the
// create modal (below the manage capability, the hint replaces the
// controls — never bait users with disabled buttons).
export function SecretList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: secrets, isPending, error } = useSecrets(machineId);
	const createSecret = useCreateSecret(machineId);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const canManage = can(machine, "manage-secrets");
	// The transfer's source gate is admin — the value leaving the machine is
	// a reveal — so the row's Move/Copy affordance sits behind
	// reveal-secrets (admin ⊆ write, so it lives in the manage cell).
	const canMove = can(machine, "reveal-secrets");

	return (
		<>
			<ResourceList
				resource="secret"
				isPending={isPending}
				error={error}
				items={secrets}
				columns={[
					{ label: "Name", sortBy: (s) => s.name },
					{ label: "Driver", sortBy: (s) => s.driver },
					{ label: "Created", sortBy: (s) => s.createdAt },
					...(canManage ? [{}] : []),
				]}
				renderRow={(secret) => (
					<SecretRow
						key={secret.id}
						machine={machine}
						secret={secret}
						canDelete={canManage}
						canMove={canMove}
					/>
				)}
				toolbar={
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => setIsCreateOpen(true)}
						data-testid="create-secret-button"
					>
						Create secret
					</Button>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<CreateSecretDialog
					mutation={createSecret}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
		</>
	);
}
