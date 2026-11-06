import { Button, HStack, Td, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CreateSecretDialog } from "@features/secrets/components/CreateSecretDialog";
import { MoveSecretDialog } from "@features/secrets/components/MoveSecretDialog";
import {
	useBulkRemoveSecrets,
	useCreateSecret,
	useSecrets,
} from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";

import { ResourceList } from "@shared/components/ResourceList";
import { detailPath } from "@shared/routes/detailRoute";
import type { Host } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

// The row is a plain function returning the <Tr>, not a component: the
// list's cell-cloning walk (mobile pinning and the selection checkbox
// column) reaches a row's cells only when `renderRow` hands back a `Tr` or a
// `Fragment` of `Tr`s — a component element is opaque to it, so a row
// wrapped in one silently loses both. The key rides the returned `Tr`, which
// is the element the list's map places. The row's own modal (move/copy)
// lives in the list, like every other list's: a plain function cannot own
// the state that opens it.
//
// The row itself is clickable: it navigates to the secret's detail page
// (spec 301), and the action inside it — the move/copy button — stops
// propagation so it never also fires the row action.
//
// The row carries no destructive button (spec 461, ticket #470): a single
// secret is removed from its detail page, several from the toolbar's "Remove
// selected".
function renderSecretRow({
	secret,
	canMove,
	onOpen,
	onMove,
}: {
	secret: SecretSummary;
	canMove: boolean;
	onOpen: (secret: SecretSummary) => void;
	onMove: (secret: SecretSummary) => void;
}) {
	return (
		<Tr
			key={secret.id}
			data-testid="secret-row"
			cursor="pointer"
			onClick={() => onOpen(secret)}
		>
			<Td fontFamily="mono">{secret.name}</Td>
			<Td>{secret.driver}</Td>
			<Td>{formatRelativeDate(secret.createdAt)}</Td>
			{canMove ? (
				<Td>
					<HStack spacing={2}>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Move or copy ${secret.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onMove(secret);
							}}
							data-testid="move-secret-button"
						>
							Move/Copy
						</Button>
					</HStack>
				</Td>
			) : null}
		</Tr>
	);
}

// The Secrets tab body. Metadata is visible to any grantee (the backend's
// read level); write affordances are gated on the host's per-caller
// facts. The toolbar/triad/table plumbing and the hint swap are
// ResourceList's; this adapter owns only the columns, the row, and the
// create/move/bulk-remove modals (below the manage capability, the hint
// replaces the controls — never bait users with disabled buttons).
export function SecretList({ host }: { host: Host }) {
	const hostId = host.id;
	const navigate = useNavigate();
	const { data: secrets, isPending, error } = useSecrets(hostId);
	const createSecret = useCreateSecret(hostId);
	const bulkRemoveSecrets = useBulkRemoveSecrets(hostId);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [moveTarget, setMoveTarget] = useState<SecretSummary | null>(null);
	// The bulk-remove dialog's snapshot: the selected secrets plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		secrets: SecretSummary[];
		clear: () => void;
	} | null>(null);
	const canManage = can(host, "manage-secrets");
	// The transfer's source gate is admin — the value leaving the host is
	// a reveal — so the row's Move/Copy affordance sits behind
	// reveal-secrets (admin ⊆ write, so it lives in the manage cell).
	const canMove = can(host, "reveal-secrets");

	return (
		<>
			<ResourceList
				resource="secret"
				isPending={isPending}
				error={error}
				items={secrets}
				columns={[
					{ label: "Name", isName: true, sortBy: (s) => s.name },
					{ label: "Driver", sortBy: (s) => s.driver },
					{ label: "Created", sortBy: (s) => s.createdAt },
					// The action column exists only when the row can offer the
					// action: Move/Copy is the one row action left (spec 461,
					// ticket #470 retired the delete button), and it is gated on
					// the admin-level reveal-secrets capability.
					...(canMove ? [{}] : []),
				]}
				renderRow={(secret) =>
					renderSecretRow({
						secret,
						canMove,
						onOpen: (target) =>
							navigate(detailPath(hostId, "secrets", target.name)),
						onMove: setMoveTarget,
					})
				}
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
				// The selection key is the secret's name, the identity the
				// removal endpoint addresses — the same one the detail
				// page's single-item Remove sends — so the bulk request and
				// the report entries speak the feature's own vocabulary.
				selection={{ getKey: (secret) => secret.name }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						secrets: selection.selectedItems,
						clear: selection.clear,
					})
				}
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.secrets}
					noun="secret"
					itemKey={(secret) => secret.name}
					itemLabel={(secret) => secret.name}
					mutation={bulkRemoveSecrets}
					force={false}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
			{isCreateOpen ? (
				<CreateSecretDialog
					mutation={createSecret}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
			{moveTarget ? (
				<MoveSecretDialog
					host={host}
					secret={moveTarget}
					onClose={() => setMoveTarget(null)}
				/>
			) : null}
		</>
	);
}
