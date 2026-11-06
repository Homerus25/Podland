import { Button, HStack } from "@chakra-ui/react";
import type { usePodAction } from "@features/pods/hooks/usePods";
import { identityKey, rowScoped } from "@shared/queries/rowScopedMutation";

// The pod action set, owned once (spec #477 ticket #479): the list row and
// the pod detail page render the same buttons from the same mutations, so
// the request shapes, the state-following labels, and the gating can never
// drift between the two surfaces. A caller owns the mutation instances —
// the list owns one per action for every row (a plain row function cannot
// own hooks), the detail page owns one per action for its single pod — and
// hands them over as this record.
export type PodActionMutations = {
	start: ReturnType<typeof usePodAction>;
	stop: ReturnType<typeof usePodAction>;
	pause: ReturnType<typeof usePodAction>;
	unpause: ReturnType<typeof usePodAction>;
	kill: ReturnType<typeof usePodAction>;
	restart: ReturnType<typeof usePodAction>;
};

// The pod's action buttons: Start/Stop and Pause/Unpause are toggles whose
// labels follow the pod's state, Kill and Restart take no parameters
// (podman's defaults), and Logs reads at read level so it renders for every
// caller. The lifecycle affordances are write-gated: `canManage` false
// renders the Logs button alone — the caller decides whether that means a
// hidden group (the list's read-only row) or the shared write hint beside it
// (the detail page's denied gate). Pending and error state are scoped by the
// mutation's own variables through the shared projection, so a list row
// shows only its own in-flight call while the detail page — whose mutations
// only ever carry its one pod's id — shows the same thing.
//
// Every button stops propagation: the list row is clickable (it opens the
// pod's detail page) and a button click must never also fire the row action.
export function PodActionButtons({
	pod,
	actions,
	canManage,
	size = "xs",
	onShowLogs,
}: {
	pod: { id: string; name: string; state: string };
	actions: PodActionMutations;
	canManage: boolean;
	size?: "xs" | "sm";
	onShowLogs: () => void;
}) {
	// Podman's pod status is capitalized ("Paused", "Running"); compare
	// case-insensitively.
	const paused = pod.state.toLowerCase() === "paused";
	const pauseAction = paused ? actions.unpause : actions.pause;
	const isRunning = pod.state.toLowerCase() === "running";
	const action = isRunning
		? {
				lifecycle: actions.stop,
				testId: "stop-pod-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: actions.start,
				testId: "start-pod-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	// The toggle's sibling mutation resets on each click, so a stale error
	// from the other action never masks the fresh one.
	const sibling = isRunning ? actions.start : actions.stop;
	const lifecycle = rowScoped(action.lifecycle, pod.id, identityKey);
	const pauseState = rowScoped(pauseAction, pod.id, identityKey);
	const killState = rowScoped(actions.kill, pod.id, identityKey);
	const restartState = rowScoped(actions.restart, pod.id, identityKey);

	return (
		<HStack spacing={2}>
			{canManage ? (
				<>
					<Button
						size={size}
						variant="outline"
						colorScheme={action.colorScheme}
						aria-label={`${action.label} ${pod.name}`}
						isLoading={lifecycle.isPending}
						onClick={(event) => {
							event.stopPropagation();
							sibling.reset();
							action.lifecycle.mutate(pod.id);
						}}
						data-testid={action.testId}
					>
						{action.label}
					</Button>
					<Button
						size={size}
						variant="outline"
						aria-label={`${paused ? "Unpause" : "Pause"} ${pod.name}`}
						isLoading={pauseState.isPending}
						onClick={(event) => {
							event.stopPropagation();
							pauseAction.mutate(pod.id);
						}}
						data-testid={paused ? "unpause-pod-button" : "pause-pod-button"}
					>
						{paused ? "Unpause" : "Pause"}
					</Button>
					<Button
						size={size}
						variant="outline"
						aria-label={`Kill ${pod.name}`}
						isLoading={killState.isPending}
						onClick={(event) => {
							event.stopPropagation();
							actions.kill.mutate(pod.id);
						}}
						data-testid="kill-pod-button"
					>
						Kill
					</Button>
					<Button
						size={size}
						variant="outline"
						aria-label={`Restart ${pod.name}`}
						isLoading={restartState.isPending}
						onClick={(event) => {
							event.stopPropagation();
							actions.restart.mutate(pod.id);
						}}
						data-testid="restart-pod-button"
					>
						Restart
					</Button>
				</>
			) : null}
			<Button
				size={size}
				variant="outline"
				aria-label={`Logs ${pod.name}`}
				onClick={(event) => {
					event.stopPropagation();
					onShowLogs();
				}}
				data-testid="logs-pod-button"
			>
				Logs
			</Button>
		</HStack>
	);
}
