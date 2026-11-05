import { Button, Text, VStack } from "@chakra-ui/react";
import { HostForm } from "@features/hosts/components/HostForm";
import { ShareHostSection } from "@features/hosts/components/ShareHostSection";
import { useDeleteHost, useUpdateHost } from "@features/hosts/hooks/useHosts";
import { can } from "@features/hosts/permissions";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { PermissionHint } from "@shared/components/PermissionHint";
import type { Host } from "@shared/types/api";
import { mutationToPromise } from "@shared/utils/mutationPromise";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

function DeleteHostSection({ host }: { host: Host }) {
	const navigate = useNavigate();
	const deleteHost = useDeleteHost();
	const [isConfirmOpen, setIsConfirmOpen] = useState(false);

	// Deleting the host destroys the page's subject, so success always
	// navigates back to the hosts list. Failure stays put: the envelope
	// renders inside the dialog, which stays open for another attempt.
	return (
		<>
			<Button
				colorScheme="red"
				variant="outline"
				onClick={() => setIsConfirmOpen(true)}
				data-testid="delete-host-button"
			>
				Delete host
			</Button>
			{isConfirmOpen ? (
				<ConfirmDialog
					mutation={deleteHost}
					title="Delete host"
					confirmLabel="Delete host"
					confirm={() => host.id}
					onClose={() => setIsConfirmOpen(false)}
					onSuccess={() => navigate("/hosts")}
					body={
						<Text>
							Delete “{host.name}” ({host.hostname}:{host.port})? This removes
							the host registration and all access grants. The remote host is
							not touched.
						</Text>
					}
				/>
			) : null}
		</>
	);
}

export function HostSettingsTab({ host }: { host: Host }) {
	const updateHost = useUpdateHost(host.id);
	// A cancelled save still runs to completion on the server; this flag
	// suppresses that late settle so neither the saved banner nor the error
	// envelope resurrects over the reverted fields. The next save clears it.
	const [cancelledSave, setCancelledSave] = useState(false);

	// The tab gates once, here. Every act it renders — metadata edit,
	// sharing, deletion — is an owner capability on the backend, so a
	// non-owner (read, write, or admin grantee) gets exactly one truthful
	// hint instead of a form whose every save is a 403.
	if (!can(host, "edit-host")) {
		return (
			<PermissionHint scope="settings" requirement="owner" phrase="edit" />
		);
	}
	// Invariant: only the host's owner renders below — the share and
	// delete sections need no further gating.
	return (
		<VStack spacing={4} align="stretch">
			{updateHost.isSuccess && !cancelledSave && (
				<Text color="green.500" data-testid="host-saved">
					Changes saved.
				</Text>
			)}
			<HostForm
				mode="edit"
				host={host}
				onSubmit={(input) => {
					setCancelledSave(false);
					return mutationToPromise(updateHost.mutate, input);
				}}
				// Cancel resets the mutation: isSuccess drops (the saved banner
				// disappears) and the error envelope clears with the fields
				// reverted by the form itself. While a save is in flight the
				// suppression flag also latches, so its late settle stays hidden.
				onCancel={() => {
					if (updateHost.isPending) setCancelledSave(true);
					updateHost.reset();
				}}
				submitError={cancelledSave ? null : updateHost.error}
				pending={updateHost.isPending}
			/>
			<Text fontWeight="bold">Sharing</Text>
			<ShareHostSection hostId={host.id} ownerId={host.ownerId} />
			<DeleteHostSection host={host} />
		</VStack>
	);
}
