import { Box, Button, Td, Text, Tr } from "@chakra-ui/react";
import {
	canManageSecrets,
	type EffectivePermission,
} from "@features/machines/permissions";
import { SecretDetailModal } from "@features/secrets/components/SecretDetailModal";
import { SecretForm } from "@features/secrets/components/SecretForm";
import {
	useCreateSecret,
	useDeleteSecret,
	useSecrets,
} from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { ResourceList } from "@shared/components/ResourceList";
import { getApiError } from "@shared/utils/api";
import { formatRelativeDate } from "@shared/utils/format";
import { mutationToPromise } from "@shared/utils/mutationPromise";
import { useState } from "react";

// No delete request leaves the browser until the user explicitly confirms.
// There are no options in v1: the backend exposes no force parameter.
function DeleteSecretModal({
	secret,
	deleteSecret,
	onClose,
}: {
	secret: SecretSummary;
	deleteSecret: ReturnType<typeof useDeleteSecret>;
	onClose: () => void;
}) {
	const confirmDelete = () => {
		deleteSecret.mutate(secret.name, { onSuccess: onClose });
	};

	return (
		<Modal
			isOpen
			onClose={onClose}
			title="Delete secret"
			footer={
				<>
					<Button variant="ghost" onClick={onClose}>
						Cancel
					</Button>
					<Button
						colorScheme="red"
						onClick={confirmDelete}
						isLoading={deleteSecret.isPending}
						data-testid="confirm-delete-button"
					>
						Delete secret
					</Button>
				</>
			}
		>
			<Text>Delete “{secret.name}”? This cannot be undone.</Text>
			{deleteSecret.error ? (
				<Box mt={2}>
					<ErrorDisplay error={getApiError(deleteSecret.error)} />
				</Box>
			) : null}
		</Modal>
	);
}

// Each row owns its delete mutation instance, so pending state and errors
// (e.g. 409 SECRET_IN_USE) stay scoped to this secret's modal and other
// rows stay usable.
function SecretRow({
	machineId,
	secret,
	permission,
	canDelete,
}: {
	machineId: string;
	secret: SecretSummary;
	permission: EffectivePermission;
	canDelete: boolean;
}) {
	const deleteSecret = useDeleteSecret(machineId);
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	const [isDetailOpen, setIsDetailOpen] = useState(false);

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
						<Button
							size="xs"
							colorScheme="red"
							variant="outline"
							aria-label={`Delete ${secret.name}`}
							onClick={() => {
								deleteSecret.reset();
								setIsDeleteOpen(true);
							}}
							data-testid="delete-secret-button"
						>
							Delete
						</Button>
					</Td>
				) : null}
			</Tr>
			{isDetailOpen ? (
				<SecretDetailModal
					machineId={machineId}
					secret={secret}
					permission={permission}
					onClose={() => setIsDetailOpen(false)}
				/>
			) : null}
			{isDeleteOpen ? (
				<DeleteSecretModal
					secret={secret}
					deleteSecret={deleteSecret}
					onClose={() => {
						deleteSecret.reset();
						setIsDeleteOpen(false);
					}}
				/>
			) : null}
		</>
	);
}

// The Secrets tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on `permission`, which the page
// derives from the machine (effectivePermission) and passes down. The
// toolbar/triad/table machinery and the hint swap are ResourceList's; this
// adapter owns only the columns, the row, and the create modal (below write,
// the hint replaces the controls — never bait users with disabled buttons).
export function SecretList({
	machineId,
	permission,
}: {
	machineId: string;
	permission: EffectivePermission;
}) {
	const { data: secrets, isPending, error } = useSecrets(machineId);
	const createSecret = useCreateSecret(machineId);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const canManage = canManageSecrets(permission);

	return (
		<>
			<ResourceList
				resource="secret"
				isPending={isPending}
				error={error}
				items={secrets}
				columns={[
					{ label: "Name" },
					{ label: "Driver" },
					{ label: "Created" },
					...(canManage ? [{}] : []),
				]}
				renderRow={(secret) => (
					<SecretRow
						key={secret.id}
						machineId={machineId}
						secret={secret}
						permission={permission}
						canDelete={canManage}
					/>
				)}
				toolbar={
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => {
							createSecret.reset();
							setIsCreateOpen(true);
						}}
						data-testid="create-secret-button"
					>
						Create secret
					</Button>
				}
				canManage={canManage}
			/>
			{isCreateOpen ? (
				<Modal
					isOpen
					onClose={() => {
						createSecret.reset();
						setIsCreateOpen(false);
					}}
					title="Create secret"
				>
					<SecretForm
						onSubmit={(input) =>
							mutationToPromise(createSecret.mutate, input, {
								onSuccess: () => setIsCreateOpen(false),
							})
						}
						submitError={createSecret.error}
						isSubmitting={createSecret.isPending}
					/>
				</Modal>
			) : null}
		</>
	);
}
