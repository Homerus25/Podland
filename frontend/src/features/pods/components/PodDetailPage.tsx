import { Box, Button, HStack, Table, Tbody, Text } from "@chakra-ui/react";
import { useCapabilityGate } from "@features/hosts/hooks/useCapabilityGate";
import { GenerateQuadletModal } from "@features/pods/components/GenerateQuadletModal";
import { POD_RUNNING_REFUSAL } from "@features/pods/components/podRemoval";
import {
	usePod,
	usePodAction,
	usePodInspect,
	useRemovePod,
} from "@features/pods/hooks/usePods";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	isNotFound,
	removalAction,
} from "@shared/components/detailKit";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PermissionHint } from "@shared/components/PermissionHint";
import { detailPath, hostTabPath } from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

// The start/stop toggle beside the overview (spec 402 ticket #407),
// mirroring the container lifecycle buttons: one button whose label
// follows the pod's state, with the sibling mutation reset on click so a
// stale error from the other action never masks the fresh one. The
// mutation invalidates the pods prefix, which covers this page's detail
// query, so a success refetches the pod's current state; a failure
// surfaces the API's error envelope above the button.
function PodLifecycleSection({
	hostId,
	podId,
	name,
	state,
}: {
	hostId: string;
	podId: string;
	name: string;
	state: string;
}) {
	const start = usePodAction(hostId, "start");
	const stop = usePodAction(hostId, "stop");
	// Podman's pod status is capitalized ("Running"); compare
	// case-insensitively, like the list's toggle.
	const isRunning = state.toLowerCase() === "running";
	const action = isRunning
		? {
				lifecycle: stop,
				testId: "stop-pod-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: start,
				testId: "start-pod-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	const sibling = isRunning ? start : stop;

	return (
		<Box mb={4}>
			{action.lifecycle.error ? (
				<Box mb={2}>
					<ErrorDisplay error={getApiError(action.lifecycle.error)} />
				</Box>
			) : null}
			<Button
				size="sm"
				variant="outline"
				colorScheme={action.colorScheme}
				aria-label={`${action.label} ${name}`}
				isLoading={action.lifecycle.isPending}
				onClick={() => {
					sibling.reset();
					action.lifecycle.mutate(podId);
				}}
				data-testid={action.testId}
			>
				{action.label}
			</Button>
		</Box>
	);
}

// The Remove action's confirm (spec 461, ticket #465): the shared
// force-refusal preset, so the first confirm sends the plain remove and
// only the POD_STATE_CONFLICT refusal swaps in the running hint and the
// force button whose retry carries force=true. The adapter keeps the domain
// vocabulary only — the refusal words come from the podRemoval module the
// list's modal reads too, and the success side effect (navigate back to the
// list tab) arrives through the preset's onSuccess seam.
function RemovePodDialog({
	pod,
	onClose,
	removePod,
	onSuccess,
}: {
	pod: { id: string; name: string };
	onClose: () => void;
	removePod: ReturnType<typeof useRemovePod>;
	onSuccess: () => void;
}) {
	return (
		<ForceConfirmDialog
			mutation={removePod}
			title="Remove pod"
			onClose={onClose}
			refusalCode={POD_RUNNING_REFUSAL.code}
			hint={POD_RUNNING_REFUSAL.hint}
			hintTestId={POD_RUNNING_REFUSAL.hintTestId}
			confirmLabel="Remove pod"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ podId: pod.id, force })}
			onSuccess={onSuccess}
			body={<Text>Remove pod “{pod.name}”? This cannot be undone.</Text>}
		/>
	);
}

// The pod detail page (spec 352): the shared layout's shell with the
// pod's detail DTO as overview rows and cross-links to the containers in
// the pod. The overview is the usePod detail query (the freshness
// factory's one-shot flavor, refetched on every visit); the raw inspect
// report fetches lazily on the first
// "Show inspect JSON" click and never on page mount.
export function PodDetailPage() {
	const { hostId, podId } = useParams<{
		hostId: string;
		podId: string;
	}>();
	const navigate = useNavigate();
	const [revealed, setRevealed] = useState(false);
	// The generate-quadlet modal (spec #402 ticket #406) and the remove
	// confirm (spec 461, ticket #465): both rendered conditionally, so mount
	// is open and unmount is close.
	const [generateOpen, setGenerateOpen] = useState(false);
	const [removeOpen, setRemoveOpen] = useState(false);
	const pod = usePod(hostId!, podId!);
	const inspectQuery = usePodInspect(hostId!, podId!, {
		enabled: revealed,
	});
	// The capability gates (the hosts feature's hook) own the
	// per-caller classification end to end: the lifecycle section and the
	// header's Remove action render from the manage-pods tri-state —
	// allowed renders the controls, denied renders the shared write hint
	// (for a read-only caller and a failed host query alike, never a silent
	// gap), pending renders neither — and the generate-quadlet modal's save
	// flag derives from the manage-quadlets answer (allowed → savable).
	const manageGate = useCapabilityGate(hostId!, "manage-pods");
	const quadletGate = useCapabilityGate(hostId!, "manage-quadlets");
	const removePod = useRemovePod(hostId!);
	const notFound = isNotFound(pod.error);

	// The Remove action rides the detail-page capability gate (the kit owns
	// the cascade): allowed renders the button, denied the shared write hint,
	// pending neither, and a not-found overview hides it outright.
	const removeAction = removalAction({
		gate: manageGate,
		notFound,
		scope: "pods",
		resource: "pod",
		testId: "remove-pod-button",
		onClick: () => setRemoveOpen(true),
	});

	return (
		<>
			<DetailPage
				title={pod.data?.name ?? podId!}
				backTo="pods"
				isPending={pod.isPending}
				error={pod.error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={removeAction}
			>
				{pod.data ? (
					<>
						{manageGate === "allowed" ? (
							<PodLifecycleSection
								hostId={hostId!}
								podId={podId!}
								name={pod.data.name}
								state={pod.data.state}
							/>
						) : null}
						{manageGate === "denied" ? (
							// The write hint in place of the lifecycle
							// buttons: a read-only caller — and a caller
							// whose host query failed — learns why the
							// controls are gone instead of a bare gap.
							<Box mb={4}>
								<PermissionHint
									scope="pod-lifecycle"
									requirement="write"
									resource="pod"
									fontSize="sm"
								/>
							</Box>
						) : null}
						<Button
							size="sm"
							variant="outline"
							mb={2}
							onClick={() => setGenerateOpen(true)}
							data-testid="generate-quadlet-button"
						>
							Generate quadlet
						</Button>
						<Table size="sm">
							<Tbody>
								<DetailRow label="ID">
									<Text fontFamily="mono" data-testid="pod-detail-id">
										{pod.data.id}
									</Text>
								</DetailRow>
								<DetailRow label="Name">
									<Text fontFamily="mono" data-testid="pod-detail-name">
										{pod.data.name}
									</Text>
								</DetailRow>
								<DetailRow label="State">
									<Text data-testid="pod-detail-state">{pod.data.state}</Text>
								</DetailRow>
								<DetailRow label="Created">
									<Text data-testid="pod-detail-created">
										{formatRelativeDate(pod.data.createdAt)}
									</Text>
								</DetailRow>
								<DetailRow label="Restart policy">
									<Text data-testid="pod-detail-restart-policy">
										{pod.data.restartPolicy || "—"}
									</Text>
								</DetailRow>
								{pod.data.labels && Object.keys(pod.data.labels).length > 0 ? (
									<DetailRow label="Labels">
										<HStack data-testid="pod-detail-labels">
											{Object.entries(pod.data.labels).map(([key, value]) => (
												<Text key={key} fontFamily="mono">
													{key}={value}
												</Text>
											))}
										</HStack>
									</DetailRow>
								) : null}
								{pod.data.mounts && pod.data.mounts.length > 0 ? (
									<DetailRow label="Mounts">
										<HStack data-testid="pod-detail-mounts">
											{pod.data.mounts.map((mount) => (
												<Text key={mount} fontFamily="mono">
													{mount}
												</Text>
											))}
										</HStack>
									</DetailRow>
								) : null}
								{pod.data.networks && pod.data.networks.length > 0 ? (
									<DetailRow label="Networks">
										<HStack data-testid="pod-detail-networks">
											{pod.data.networks.map((network) => (
												<Text key={network} fontFamily="mono">
													{network}
												</Text>
											))}
										</HStack>
									</DetailRow>
								) : null}
								<DetailRow label="Containers">
									<HStack data-testid="pod-detail-containers">
										{pod.data.containers.map((container) => (
											// Cross-link to the container's detail page (spec 301):
											// the pod's containers are first-class resources.
											<Link
												key={container.id}
												to={detailPath(hostId!, "containers", container.id)}
												color="blue.500"
												data-testid="pod-detail-container-link"
											>
												{container.name} ({container.state})
											</Link>
										))}
										{pod.data.containers.length === 0 ? (
											<Text color="gray.500">No containers in this pod.</Text>
										) : null}
									</HStack>
								</DetailRow>
							</Tbody>
						</Table>
					</>
				) : null}
			</DetailPage>
			{removeOpen && pod.data ? (
				<RemovePodDialog
					pod={pod.data}
					onClose={() => setRemoveOpen(false)}
					removePod={removePod}
					// The removed pod's page would 404 on the next refetch, so the
					// page leaves for the list tab; the mutation's own invalidation
					// refetches that list.
					onSuccess={() => navigate(hostTabPath(hostId!, "pods"))}
				/>
			) : null}
			{generateOpen ? (
				<GenerateQuadletModal
					hostId={hostId!}
					podId={podId!}
					podName={pod.data?.name ?? podId!}
					// Save posts to the write-gated quadlet create endpoint,
					// so it hides below the manage-quadlets capability —
					// the same capability the quadlets list gates its
					// toolbar with. Generate and copy stay read-level.
					canSave={quadletGate === "allowed"}
					onClose={() => setGenerateOpen(false)}
				/>
			) : null}
		</>
	);
}
