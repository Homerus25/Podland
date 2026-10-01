import { Button, Checkbox, Text, VStack } from "@chakra-ui/react";
import { can } from "@features/machines/permissions";
import { AutoUpdateModal } from "@features/quadlets/components/AutoUpdateModal";
import {
	CreateQuadletDialog,
	EditQuadletDialog,
} from "@features/quadlets/components/QuadletDialog";
import { QuadletRow } from "@features/quadlets/components/QuadletRow";
import { useQuadletAutoUpdate } from "@features/quadlets/hooks/useQuadletAutoUpdate";
import {
	useCreateQuadlet,
	useDeleteQuadlet,
	useQuadlets,
	useUpdateQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { ActionDialog, ConfirmDialog } from "@shared/components/MutationDialog";
import { ResourceList } from "@shared/components/ResourceList";
import { useOpenLogs } from "@shared/logs/logRoute";
import type { Machine } from "@shared/types/api";
import { useState } from "react";

// No delete request leaves the browser until the user explicitly confirms.
// The danger footer, pending wiring, close-on-success, reset lifecycle, and
// error region are ConfirmDialog's; this adapter owns the domain body (the
// two option checkboxes) and the variables: stop-first defaults to on and
// force to off, matching the backend contract's defaults, so confirming
// without touching anything behaves like the API's own defaults.
function DeleteQuadletModal({
	quadlet,
	deleteQuadlet,
	onClose,
}: {
	quadlet: QuadletSummary;
	deleteQuadlet: ReturnType<typeof useDeleteQuadlet>;
	onClose: () => void;
}) {
	const [stopFirst, setStopFirst] = useState(true);
	const [force, setForce] = useState(false);

	return (
		<ConfirmDialog
			mutation={deleteQuadlet}
			title="Delete quadlet"
			confirmLabel="Delete quadlet"
			confirm={() => ({ quadletId: quadlet.id, stop: stopFirst, force })}
			onClose={onClose}
			body={
				<>
					<Text>Delete “{quadlet.name}”? This cannot be undone.</Text>
					<VStack align="flex-start" mt={3} spacing={2}>
						<Checkbox
							isChecked={stopFirst}
							onChange={(event) => setStopFirst(event.target.checked)}
							data-testid="stop-first-option"
						>
							Stop the unit before deleting
						</Checkbox>
						<Checkbox
							isChecked={force}
							onChange={(event) => setForce(event.target.checked)}
							data-testid="force-option"
						>
							Force
						</Checkbox>
					</VStack>
				</>
			}
		/>
	);
}

// The toolbar/triad/table machinery, the plural testids, and the hint swap
// are ResourceList's; this adapter owns the busiest toolbar of the five
// (create plus the two auto-update buttons), the edit/delete/logs/auto-update
// feature code, and the now-tiny update-all pre-flight (an ActionDialog: the
// confirm gates the caller-owned stream, so no mutation machinery applies).
// The row and its lifecycle options modal live in QuadletRow. Create is a
// write like every other toolbar action, so below manage level the module
// swaps the whole toolbar for the canonical read hint.
export function QuadletList({ machine }: { machine: Machine }) {
	const machineId = machine.id;
	const { data: quadlets, isPending, error } = useQuadlets(machineId);
	const createQuadlet = useCreateQuadlet(machineId);
	const updateQuadlet = useUpdateQuadlet(machineId);
	const deleteQuadlet = useDeleteQuadlet(machineId);
	const autoUpdate = useQuadletAutoUpdate(machineId);
	const canManage = can(machine, "manage-quadlets");
	const [isConfirmUpdateOpen, setIsConfirmUpdateOpen] = useState(false);
	const [isAutoUpdateOpen, setIsAutoUpdateOpen] = useState(false);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [editTarget, setEditTarget] = useState<QuadletSummary | null>(null);
	const [deleteTarget, setDeleteTarget] = useState<QuadletSummary | null>(null);
	const openLogs = useOpenLogs(machineId, "quadlet");

	return (
		<>
			<ResourceList
				resource="quadlet"
				isPending={isPending}
				error={error}
				items={quadlets}
				columns={[
					{ label: "Name" },
					{ label: "Resource" },
					{ label: "State" },
					{},
					{},
					{},
				]}
				renderRow={(row) => (
					<QuadletRow
						key={row.id}
						machineId={machineId}
						quadlet={row}
						onEdit={setEditTarget}
						onDelete={setDeleteTarget}
						onShowLogs={(quadlet) =>
							openLogs({ targetId: quadlet.id, name: quadlet.name })
						}
					/>
				)}
				toolbar={
					<>
						<Button
							size="sm"
							colorScheme="blue"
							onClick={() => setIsCreateOpen(true)}
							data-testid="create-quadlet-button"
						>
							Create quadlet
						</Button>
						<Button
							size="sm"
							colorScheme="purple"
							onClick={() => {
								autoUpdate.run(true);
								setIsAutoUpdateOpen(true);
							}}
							data-testid="check-updates-button"
						>
							Check for updates
						</Button>
						<Button
							size="sm"
							colorScheme="orange"
							onClick={() => setIsConfirmUpdateOpen(true)}
							data-testid="update-all-button"
						>
							Update all
						</Button>
					</>
				}
				canManage={canManage}
			/>
			{editTarget ? (
				<EditQuadletDialog
					machineId={machineId}
					quadlet={editTarget}
					mutation={updateQuadlet}
					onClose={() => setEditTarget(null)}
				/>
			) : null}
			{deleteTarget ? (
				<DeleteQuadletModal
					quadlet={deleteTarget}
					deleteQuadlet={deleteQuadlet}
					onClose={() => setDeleteTarget(null)}
				/>
			) : null}
			{isConfirmUpdateOpen ? (
				<ActionDialog
					title="Update all"
					confirmLabel="Update all"
					confirmTestId="confirm-auto-update-button"
					confirmColorScheme="orange"
					onConfirm={() => {
						setIsConfirmUpdateOpen(false);
						autoUpdate.run(false);
						setIsAutoUpdateOpen(true);
					}}
					onClose={() => setIsConfirmUpdateOpen(false)}
					body={
						<Text>
							Containers with an auto-update policy will be updated and
							restarted. Continue?
						</Text>
					}
				/>
			) : null}
			{isAutoUpdateOpen ? (
				<AutoUpdateModal
					dryRun={autoUpdate.dryRun}
					autoUpdate={autoUpdate}
					onClose={() => {
						if (autoUpdate.isRunning) autoUpdate.cancel();
						setIsAutoUpdateOpen(false);
					}}
				/>
			) : null}
			{isCreateOpen ? (
				<CreateQuadletDialog
					mutation={createQuadlet}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
		</>
	);
}
