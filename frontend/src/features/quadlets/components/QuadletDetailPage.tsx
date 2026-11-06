import {
	Box,
	Button,
	HStack,
	Table,
	Tbody,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { useHost } from "@features/hosts/hooks/useHosts";
import { MoveQuadletDialog } from "@features/quadlets/components/MoveQuadletDialog";
import { EditQuadletDialog } from "@features/quadlets/components/QuadletDialog";
import {
	LifecycleOptionsModal,
	type QuadletLifecycleMutations,
} from "@features/quadlets/components/QuadletRow";
import {
	useDeleteQuadlet,
	useMoveQuadlet,
	useQuadlet,
	useRestartQuadlet,
	useStartQuadlet,
	useStopQuadlet,
	useUpdateQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import type { QuadletRef } from "@features/quadlets/services/quadletsApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	isNotFound,
	removalAction,
} from "@shared/components/detailKit";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { StateBadge } from "@shared/components/StateBadge";
import { useOpenLogs } from "@shared/logs/logRoute";
import { hostTabPath } from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

// The detail page's Remove adapter over the plain confirm preset (issue
// #472): the single-item DELETE has no reachable refusal code under its
// defaults (stop=true, force=false — a running resource is stopped first,
// never refused), so there is no force path to offer and the module's
// default error region answers a failure in place while the page stays
// mounted. The secret detail page's no-force remove adapter is the
// precedent.
function RemoveQuadletDialog({
	quadlet,
	removeQuadlet,
	onClose,
	onRemoved,
}: {
	quadlet: QuadletRef;
	removeQuadlet: ReturnType<typeof useDeleteQuadlet>;
	onClose: () => void;
	onRemoved: () => void;
}) {
	return (
		<ConfirmDialog
			mutation={removeQuadlet}
			title="Remove quadlet"
			confirmLabel="Remove quadlet"
			confirm={() => quadlet.id}
			onClose={onClose}
			onSuccess={onRemoved}
			body={<Text>Remove “{quadlet.name}”? This cannot be undone.</Text>}
		/>
	);
}

// The quadlet detail page (issue #472): the shared layout's shell with the
// quadlet's metadata as overview rows, the quadlet file's content as primary
// page content — always visible in a monospace block, never hidden behind a
// reveal — and every action the list's row offers, riding the same
// machinery: the lifecycle options modal for stop/restart, the edit dialog,
// and the move dialog. Quadlets have no podman inspect report, so the page
// passes no inspect slice and the layout renders no reveal at all. The
// overview is the useQuadlet detail query (the freshness factory's one-shot
// flavor, refetched on every visit); the header's Remove action rides the
// detail kit's gate cascade on the manage-quadlets capability.
export function QuadletDetailPage() {
	const { hostId, quadletId } = useParams<{
		hostId: string;
		quadletId: string;
	}>();
	const quadlet = useQuadlet(hostId!, quadletId!);
	const gate = useCapabilityGate(hostId!, "manage-quadlets");
	// The move dialog needs the host itself (its transfer state filters
	// targets by the caller's capability); the gate's hook rides the same
	// host query, so this adds no request.
	const host = useHost(hostId!);
	const navigate = useNavigate();
	const notFound = isNotFound(quadlet.error);

	// The same lifecycle mutations the list rides: their list-key
	// invalidation is prefix-based and the detail key extends the list key,
	// so a successful start/stop/restart refetches this page's overview too
	// and the state badge reflects the host's report without a manual
	// reload.
	const lifecycle: QuadletLifecycleMutations = {
		start: useStartQuadlet(hostId!),
		stop: useStopQuadlet(hostId!),
		restart: useRestartQuadlet(hostId!),
	};
	const updateQuadlet = useUpdateQuadlet(hostId!);
	const moveQuadlet = useMoveQuadlet(hostId!);
	const removeQuadlet = useDeleteQuadlet(hostId!);
	const openLogs = useOpenLogs(hostId!, "quadlet");

	// Stop and restart both run through the options modal; this remembers
	// which one is open (null = none), exactly like the list.
	const [lifecycleAction, setLifecycleAction] = useState<
		"stop" | "restart" | null
	>(null);
	const [editing, setEditing] = useState(false);
	const [moving, setMoving] = useState(false);
	const [removing, setRemoving] = useState(false);

	const removeAction = removalAction({
		gate,
		notFound,
		scope: "quadlets",
		resource: "quadlet",
		testId: "remove-quadlet-button",
		onClick: () => setRemoving(true),
	});

	const data = quadlet.data;
	const isRunning = data?.state === "running";
	const lifecyclePending =
		lifecycle.start.isPending ||
		lifecycle.stop.isPending ||
		lifecycle.restart.isPending;
	// Only a failed start reports here; stop and restart report inside
	// their still-open options modal (the row's convention).
	const startError = lifecycle.start.error;

	return (
		<>
			<DetailPage
				title={data?.name ?? quadletId!}
				backTo="quadlets"
				isPending={quadlet.isPending}
				error={quadlet.error}
				notFound={notFound}
				headerActions={removeAction}
			>
				{data ? (
					<VStack align="stretch" spacing={4}>
						{/* The row's action set, page-sized: Start/Stop by state,
						    Restart, and Move/Copy ride the capability gate's
						    tri-state (issue #484) — allowed renders the control,
						    pending renders neither, and denied hides it behind
						    the page's one write hint, which the header's Remove
						    cascade already renders for the same capability (the
						    one-hint rule), so these actions carry no hint of
						    their own. Logs and Edit are unchanged: read-level
						    here, they render for every caller. */}
						<HStack spacing={2} flexWrap="wrap">
							{gate === "allowed" ? (
								<>
									{!isRunning ? (
										<Button
											size="sm"
											variant="outline"
											colorScheme="green"
											aria-label={`Start ${data.name}`}
											onClick={() => lifecycle.start.mutate(data.id)}
											isLoading={lifecycle.start.isPending}
											isDisabled={lifecyclePending}
											data-testid="start-quadlet-button"
										>
											Start
										</Button>
									) : null}
									{isRunning ? (
										<Button
											size="sm"
											variant="outline"
											aria-label={`Stop ${data.name}`}
											onClick={() => setLifecycleAction("stop")}
											isDisabled={lifecyclePending}
											data-testid="stop-quadlet-button"
										>
											Stop
										</Button>
									) : null}
									<Button
										size="sm"
										variant="outline"
										aria-label={`Restart ${data.name}`}
										onClick={() => setLifecycleAction("restart")}
										isDisabled={lifecyclePending}
										data-testid="restart-quadlet-button"
									>
										Restart
									</Button>
								</>
							) : null}
							<Button
								size="sm"
								variant="outline"
								aria-label={`Logs ${data.name}`}
								onClick={() => openLogs({ targetId: data.id, name: data.name })}
							>
								Logs
							</Button>
							<Button
								size="sm"
								variant="outline"
								aria-label={`Edit ${data.name}`}
								onClick={() => setEditing(true)}
								data-testid="edit-quadlet-button"
							>
								Edit
							</Button>
							{gate === "allowed" ? (
								<Button
									size="sm"
									variant="outline"
									aria-label={`Move or copy ${data.name}`}
									onClick={() => setMoving(true)}
									data-testid="move-quadlet-button"
								>
									Move/Copy
								</Button>
							) : null}
						</HStack>
						{startError ? (
							<ErrorDisplay error={getApiError(startError)} />
						) : null}
						<Box>
							<Table size="sm">
								<Tbody>
									<DetailRow label="Name">
										<Text fontFamily="mono" data-testid="quadlet-detail-name">
											{data.name}
										</Text>
									</DetailRow>
									<DetailRow label="Resource">
										<Text
											fontFamily="mono"
											data-testid="quadlet-detail-resource"
										>
											{data.resource}
										</Text>
									</DetailRow>
									<DetailRow label="State">
										<StateBadge state={data.state} />
									</DetailRow>
									<DetailRow label="Path">
										<Text fontFamily="mono" data-testid="quadlet-detail-path">
											{data.path}
										</Text>
									</DetailRow>
									<DetailRow label="Unit">
										<Text fontFamily="mono" data-testid="quadlet-detail-unit">
											{data.unit}
										</Text>
									</DetailRow>
								</Tbody>
							</Table>
						</Box>
						<Box>
							<Text fontWeight="bold">File content</Text>
							<Box
								as="pre"
								mt={2}
								overflow="auto"
								maxHeight="60vh"
								fontFamily="mono"
								data-testid="quadlet-file-content"
							>
								{data.content}
							</Box>
						</Box>
					</VStack>
				) : null}
			</DetailPage>
			{lifecycleAction && data ? (
				<LifecycleOptionsModal
					quadlet={data}
					actionLabel={lifecycleAction === "stop" ? "Stop" : "Restart"}
					lifecycle={
						lifecycleAction === "stop" ? lifecycle.stop : lifecycle.restart
					}
					onClose={() => setLifecycleAction(null)}
				/>
			) : null}
			{editing && data ? (
				<EditQuadletDialog
					hostId={hostId!}
					quadlet={data}
					mutation={updateQuadlet}
					onClose={() => setEditing(false)}
				/>
			) : null}
			{moving && data && host.data ? (
				<MoveQuadletDialog
					host={host.data}
					quadlet={data}
					moveQuadlet={moveQuadlet}
					onClose={() => setMoving(false)}
				/>
			) : null}
			{removing && data ? (
				<RemoveQuadletDialog
					quadlet={data}
					removeQuadlet={removeQuadlet}
					onClose={() => setRemoving(false)}
					onRemoved={() => navigate(hostTabPath(hostId!, "quadlets"))}
				/>
			) : null}
		</>
	);
}
