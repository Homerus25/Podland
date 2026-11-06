import { Button, HStack } from "@chakra-ui/react";
import type {
	useRestartContainer,
	useStartContainer,
	useStopContainer,
} from "@features/containers/hooks/useContainers";
import { identityKey, rowScoped } from "@shared/queries/rowScopedMutation";

// The container action set, owned once (spec #477): the list row and the
// container detail page render the same buttons from the same mutations, so
// the request shapes, the state-following labels, and the two gates can
// never drift between the two surfaces. A caller owns the mutation
// instances — the list owns one per action for every row (a plain row
// function cannot own hooks), the detail page owns one per action for its
// single container — and hands them over as this record.
export type ContainerLifecycleMutations = {
	start: ReturnType<typeof useStartContainer>;
	stop: ReturnType<typeof useStopContainer>;
	restart: ReturnType<typeof useRestartContainer>;
};

// The container's action buttons: Start/Stop is a toggle whose label follows
// the container's state, Restart is additive and renders beside it only
// while the container is running (issue #384), Connect to network rides the
// separate manage-networks capability (issue 256), and Logs reads at read
// level so it renders for every caller. The lifecycle affordances are
// write-gated: `canManage` false renders the Logs button alone — the caller
// decides whether that means a hidden group (the list's read-only row) or
// the shared write hint beside it (the detail page's denied gate). Pending
// and error state are scoped by the mutation's own variables through the
// shared projection, so a list row shows only its own in-flight call while
// the detail page — whose mutations only ever carry its one container's id —
// shows the same thing.
//
// Every button stops propagation: the list row is clickable (it opens the
// container's detail page) and a button click must never also fire the row
// action.
export function ContainerActionButtons({
	container,
	name,
	lifecycle,
	canManage,
	canManageNetworks,
	size = "xs",
	onConnect,
	onShowLogs,
}: {
	container: { id: string; state: string };
	// The display name the caller owns: the row derives it from the
	// summary's `names`, the detail page from the detail DTO's `name`.
	name: string;
	lifecycle: ContainerLifecycleMutations;
	canManage: boolean;
	canManageNetworks: boolean;
	size?: "xs" | "sm";
	onConnect: () => void;
	onShowLogs: () => void;
}) {
	const isRunning = container.state === "running";
	const action = isRunning
		? {
				lifecycle: lifecycle.stop,
				testId: "stop-container-button",
				label: "Stop",
				colorScheme: undefined,
			}
		: {
				lifecycle: lifecycle.start,
				testId: "start-container-button",
				label: "Start",
				colorScheme: "green" as const,
			};
	// The toggle's sibling mutation resets on each click, so a stale error
	// from the other action never masks the fresh one.
	const sibling = isRunning ? lifecycle.start : lifecycle.stop;
	const actionScoped = rowScoped(action.lifecycle, container.id, identityKey);
	const restartScoped = rowScoped(lifecycle.restart, container.id, identityKey);

	return (
		<HStack spacing={2}>
			{canManage ? (
				<>
					<Button
						size={size}
						variant="outline"
						colorScheme={action.colorScheme}
						onClick={(event) => {
							event.stopPropagation();
							sibling.reset();
							action.lifecycle.mutate(container.id);
						}}
						isLoading={actionScoped.isPending}
						data-testid={action.testId}
					>
						{action.label}
					</Button>
					{isRunning ? (
						<Button
							size={size}
							variant="outline"
							aria-label={`Restart ${name}`}
							onClick={(event) => {
								event.stopPropagation();
								action.lifecycle.reset();
								lifecycle.restart.mutate(container.id);
							}}
							isLoading={restartScoped.isPending}
							data-testid="restart-container-button"
						>
							Restart
						</Button>
					) : null}
				</>
			) : null}
			{canManageNetworks ? (
				<Button
					size={size}
					variant="outline"
					aria-label={`Connect to network ${name}`}
					onClick={(event) => {
						event.stopPropagation();
						onConnect();
					}}
					data-testid="connect-container-button"
				>
					Connect to network
				</Button>
			) : null}
			<Button
				size={size}
				variant="outline"
				aria-label={`Logs ${name}`}
				onClick={(event) => {
					event.stopPropagation();
					onShowLogs();
				}}
				data-testid="logs-container-button"
			>
				Logs
			</Button>
		</HStack>
	);
}
