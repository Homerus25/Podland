import { Button, Td, Tr } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { CreateSecretDialog } from "@features/secrets/components/CreateSecretDialog";
import { MoveSecretsDialog } from "@features/secrets/components/MoveSecretsDialog";
import {
	useBulkRemoveSecrets,
	useCreateSecret,
	useSecrets,
} from "@features/secrets/hooks/useSecrets";
import type { SecretSummary } from "@features/secrets/services/secretsApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";

import {
	ResourceList,
	type ResourceListSelectionSnapshot,
	snapshotSelection,
} from "@shared/components/ResourceList";
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
// is the element the list's map places.
//
// The row itself is clickable: it navigates to the secret's detail page
// (spec 301).
//
// The row carries no action button at all (spec 461, ticket #470 retired the
// delete button; spec 486, ticket #487 retired Move/Copy): a single secret is
// removed and moved from its detail page, several from the toolbar's bulk
// actions. The action column went with the last button.
function renderSecretRow({
	secret,
	onOpen,
}: {
	secret: SecretSummary;
	onOpen: (secret: SecretSummary) => void;
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
	// The bulk-remove dialog's snapshot: the selected secrets plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		secrets: SecretSummary[];
		clear: () => void;
	} | null>(null);
	// The batch transfer's snapshot, the same shape: the selected secrets
	// plus the selection's partial reset. A transfer clears only the secrets
	// that succeeded, so the failures stay selected for the retry.
	const [moveSelectedTarget, setMoveSelectedTarget] =
		useState<ResourceListSelectionSnapshot<SecretSummary> | null>(null);
	const canManage = can(host, "manage-secrets");
	// A secret's value leaving the host is a reveal, so the backend refuses
	// the transfer below admin while the toolbar's write gate is
	// manage-secrets. The button therefore renders for every write-capable
	// caller — permanently, like every selection action — and a caller
	// without reveal-secrets gets it disabled with the requirement named,
	// rather than hidden or baited into a 403.
	const canRevealSecrets = can(host, "reveal-secrets");

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
				]}
				renderRow={(secret) =>
					renderSecretRow({
						secret,
						onOpen: (target) =>
							navigate(detailPath(hostId, "secrets", target.name)),
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
				// removal and transfer endpoints address — the same one the
				// detail page's single-item actions send — so the bulk requests
				// and the report entries speak the feature's own vocabulary.
				selection={{ getKey: (secret) => secret.name }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						secrets: selection.selectedItems,
						clear: selection.clear,
					})
				}
				selectionActions={(selection) => (
					<Button
						size="sm"
						variant="outline"
						isDisabled={
							!canRevealSecrets || selection.selectedKeys.length === 0
						}
						title={
							canRevealSecrets
								? undefined
								: "Moving a secret requires admin access"
						}
						onClick={() => setMoveSelectedTarget(snapshotSelection(selection))}
						data-testid="move-selected-secrets-button"
					>
						Move/copy selected
					</Button>
				)}
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
			{moveSelectedTarget ? (
				<MoveSecretsDialog
					host={host}
					secrets={moveSelectedTarget.items}
					// Only the succeeded secrets leave the selection.
					onFinished={moveSelectedTarget.clearKeys}
					onClose={() => setMoveSelectedTarget(null)}
				/>
			) : null}
		</>
	);
}
