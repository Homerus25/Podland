import { Button, Text } from "@chakra-ui/react";
import { can } from "@features/hosts/permissions";
import { AutoUpdateModal } from "@features/quadlets/components/AutoUpdateModal";
import { ImportComposeModal } from "@features/quadlets/components/ImportComposeModal";
import { MoveQuadletDialog } from "@features/quadlets/components/MoveQuadletDialog";
import {
	CreateQuadletDialog,
	EditQuadletDialog,
} from "@features/quadlets/components/QuadletDialog";
import {
	LifecycleOptionsModal,
	type QuadletLifecycleMutations,
	renderQuadletRow,
} from "@features/quadlets/components/QuadletRow";
import { useQuadletAutoUpdate } from "@features/quadlets/hooks/useQuadletAutoUpdate";
import {
	useBulkRemoveQuadlets,
	useCreateQuadlet,
	useMoveQuadlet,
	useQuadlets,
	useRestartQuadlet,
	useStartQuadlet,
	useStopQuadlet,
	useUpdateQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { BulkRemoveDialog } from "@shared/components/BulkRemoveDialog";
import { ActionDialog } from "@shared/components/MutationDialog";

import { ResourceList } from "@shared/components/ResourceList";
import { useOpenLogs } from "@shared/logs/logRoute";
import type { Host } from "@shared/types/api";
import { useState } from "react";

// The toolbar/triad/table plumbing, the plural testids, and the hint swap
// are ResourceList's; this adapter owns the busiest toolbar of the five
// (create plus the two auto-update buttons), the edit/logs/auto-update and
// move/copy feature code, and the now-tiny update-all pre-flight (an
// ActionDialog: the confirm gates the caller-owned stream, so no mutation
// plumbing applies).
// The row and its lifecycle options modal live in QuadletRow; the transfer
// dialog (mode, target, volume warning) lives in MoveQuadletDialog. Create is
// a write like every other toolbar action, so below manage level the module
// swaps the whole toolbar for the canonical read hint.
//
// Removal has no per-row button (spec 461, ticket #470), and quadlets have no
// detail page either: a single quadlet is removed by selecting it and using
// the toolbar's "Remove selected", exactly like a bulk selection.
export function QuadletList({ host }: { host: Host }) {
	const hostId = host.id;
	const { data: quadlets, isPending, error } = useQuadlets(hostId);
	const createQuadlet = useCreateQuadlet(hostId);
	const updateQuadlet = useUpdateQuadlet(hostId);
	const bulkRemoveQuadlets = useBulkRemoveQuadlets(hostId);
	const moveQuadlet = useMoveQuadlet(hostId);
	const autoUpdate = useQuadletAutoUpdate(hostId);
	// One instance per lifecycle action, shared by every row: a plain row
	// function cannot own hooks, so the list keeps them and each row scopes
	// its own pending state off the mutation's variables.
	const lifecycle: QuadletLifecycleMutations = {
		start: useStartQuadlet(hostId),
		stop: useStopQuadlet(hostId),
		restart: useRestartQuadlet(hostId),
	};
	const canManage = can(host, "manage-quadlets");
	const [isConfirmUpdateOpen, setIsConfirmUpdateOpen] = useState(false);
	const [isAutoUpdateOpen, setIsAutoUpdateOpen] = useState(false);
	// How the auto-update modal was opened: the check button starts a dry run,
	// the update-all confirm starts a real run. The hook's state after an
	// in-modal run switch is identical to a toolbar-opened real run, so the
	// modal cannot derive this — the list tracks it and hands it down.
	const [openedAsCheck, setOpenedAsCheck] = useState(false);
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isImportOpen, setIsImportOpen] = useState(false);
	const [editTarget, setEditTarget] = useState<QuadletSummary | null>(null);
	const [moveTarget, setMoveTarget] = useState<QuadletSummary | null>(null);
	// Stop and restart both run through the options modal; this remembers
	// which one is open and for which quadlet (null = none). The row names
	// the action, the list mounts the modal.
	const [lifecycleTarget, setLifecycleTarget] = useState<{
		quadlet: QuadletSummary;
		action: "stop" | "restart";
	} | null>(null);
	// The bulk-remove dialog's snapshot: the selected quadlets plus the
	// selection's own clear, captured when the dialog opens. The list owns
	// the selection, so the adapter never keeps a second copy of it — it
	// holds the snapshot the run reports on and hands the clear back on
	// close.
	const [bulkRemoveTarget, setBulkRemoveTarget] = useState<{
		quadlets: QuadletSummary[];
		clear: () => void;
	} | null>(null);
	const openLogs = useOpenLogs(hostId, "quadlet");

	return (
		<>
			<ResourceList
				resource="quadlet"
				isPending={isPending}
				error={error}
				items={quadlets}
				columns={[
					{ label: "Name", isName: true, sortBy: (q) => q.name },
					{ label: "Resource", sortBy: (q) => q.resource },
					{ label: "State", sortBy: (q) => q.state },
					{},
					{},
					// The move/copy column exists only when the rows can offer the
					// action (the networks list's conditional connect column is
					// the precedent), so the header matches the row either way.
					...(canManage ? [{}] : []),
				]}
				renderRow={(quadlet) =>
					renderQuadletRow({
						quadlet,
						canManage,
						lifecycle,
						onEdit: setEditTarget,
						onMove: setMoveTarget,
						onShowLogs: (target) =>
							openLogs({ targetId: target.id, name: target.name }),
						onLifecycle: (target, action) =>
							setLifecycleTarget({ quadlet: target, action }),
					})
				}
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
							colorScheme="teal"
							onClick={() => setIsImportOpen(true)}
							data-testid="import-compose-button"
						>
							Import compose file
						</Button>
						<Button
							size="sm"
							colorScheme="purple"
							onClick={() => {
								setOpenedAsCheck(true);
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
				selection={{ getKey: (quadlet) => quadlet.id }}
				onRemoveSelected={(selection) =>
					setBulkRemoveTarget({
						quadlets: selection.selectedItems,
						clear: selection.clear,
					})
				}
			/>
			{bulkRemoveTarget ? (
				<BulkRemoveDialog
					items={bulkRemoveTarget.quadlets}
					noun="quadlet"
					itemKey={(quadlet) => quadlet.id}
					itemLabel={(quadlet) => quadlet.name}
					mutation={bulkRemoveQuadlets}
					// The selection clears when the run succeeds, not when the
					// dialog closes: a failed run keeps it for the retry.
					onSuccess={bulkRemoveTarget.clear}
					onClose={() => setBulkRemoveTarget(null)}
				/>
			) : null}
			{lifecycleTarget ? (
				<LifecycleOptionsModal
					quadlet={lifecycleTarget.quadlet}
					actionLabel={lifecycleTarget.action === "stop" ? "Stop" : "Restart"}
					lifecycle={
						lifecycleTarget.action === "stop"
							? lifecycle.stop
							: lifecycle.restart
					}
					onClose={() => setLifecycleTarget(null)}
				/>
			) : null}
			{editTarget ? (
				<EditQuadletDialog
					hostId={hostId}
					quadlet={editTarget}
					mutation={updateQuadlet}
					onClose={() => setEditTarget(null)}
				/>
			) : null}
			{moveTarget ? (
				<MoveQuadletDialog
					host={host}
					quadlet={moveTarget}
					moveQuadlet={moveQuadlet}
					onClose={() => setMoveTarget(null)}
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
						setOpenedAsCheck(false);
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
					openedAsCheck={openedAsCheck}
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
			{isImportOpen ? (
				<ImportComposeModal
					hostId={hostId}
					onClose={() => setIsImportOpen(false)}
				/>
			) : null}
		</>
	);
}
