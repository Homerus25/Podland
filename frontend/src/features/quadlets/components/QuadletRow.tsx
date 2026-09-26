import {
	Button,
	FormControl,
	FormLabel,
	HStack,
	Input,
	Td,
	Text,
	Tr,
} from "@chakra-ui/react";
import {
	useRestartQuadlet,
	useStartQuadlet,
	useStopQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import type { QuadletSummary } from "@features/quadlets/services/quadletsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { StateBadge } from "@shared/components/StateBadge";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";

// Stop and restart share the backend's option set (timeout only — there is
// no force param, issue 205), so one modal serves both: the caller names the
// action and owns the mutation. Defaults mirror the backend's own (timeout
// unset and therefore omitted from the request), so confirming untouched
// sends no query parameters. The gray confirm opts out of the preset's
// destructive red — stopping a unit is routine, not danger — and the footer
// machinery (reset lifecycle, close-on-success, pending wiring, error region
// above the body) is the module's.
function LifecycleOptionsModal({
	quadlet,
	actionLabel,
	lifecycle,
	onClose,
}: {
	quadlet: QuadletSummary;
	actionLabel: string;
	lifecycle:
		| ReturnType<typeof useStopQuadlet>
		| ReturnType<typeof useRestartQuadlet>;
	onClose: () => void;
}) {
	const [timeoutSeconds, setTimeoutSeconds] = useState("");

	return (
		<ConfirmDialog
			mutation={lifecycle}
			title={`${actionLabel} quadlet`}
			confirmLabel={`${actionLabel} quadlet`}
			confirmColorScheme="gray"
			confirmTestId="confirm-lifecycle-button"
			confirm={() => {
				const timeout = Number(timeoutSeconds);
				return {
					quadletId: quadlet.id,
					// An empty input means "systemd default", so no timeout parameter
					// travels; a non-positive or otherwise unparsable value is dropped
					// for the same reason the backend drops it.
					timeout: timeout > 0 ? timeout : undefined,
				};
			}}
			onClose={onClose}
			body={
				<>
					<Text>
						{actionLabel} “{quadlet.name}”?
					</Text>
					<FormControl mt={3}>
						<FormLabel htmlFor="lifecycle-timeout">Timeout (seconds)</FormLabel>
						<Input
							id="lifecycle-timeout"
							type="number"
							min={1}
							value={timeoutSeconds}
							onChange={(event) => setTimeoutSeconds(event.target.value)}
							placeholder="systemd default"
							data-testid="lifecycle-timeout-input"
						/>
					</FormControl>
				</>
			}
		/>
	);
}

// Each row owns its lifecycle mutation instances, so pending state and
// errors are scoped to the affected quadlet and other rows stay usable.
export function QuadletRow({
	machineId,
	quadlet,
	onEdit,
	onDelete,
	onShowLogs,
}: {
	machineId: string;
	quadlet: QuadletSummary;
	onEdit: (quadlet: QuadletSummary) => void;
	onDelete: (quadlet: QuadletSummary) => void;
	onShowLogs: (quadlet: QuadletSummary) => void;
}) {
	const startQuadlet = useStartQuadlet(machineId);
	const stopQuadlet = useStopQuadlet(machineId);
	const restartQuadlet = useRestartQuadlet(machineId);
	// Stop and restart both run through the options modal; this remembers
	// which one is open (null = none).
	const [pendingAction, setPendingAction] = useState<"stop" | "restart" | null>(
		null,
	);
	// Any in-flight lifecycle action takes the row's other lifecycle buttons
	// with it, so slow systemd operations don't invite double clicks. Other
	// rows keep their own mutation instances and stay usable.
	const lifecyclePending =
		startQuadlet.isPending || stopQuadlet.isPending || restartQuadlet.isPending;

	const actions = {
		stop: { label: "Stop", mutation: stopQuadlet },
		restart: { label: "Restart", mutation: restartQuadlet },
	};
	const openAction = pendingAction ? actions[pendingAction] : null;

	// The modal mounts open and unmounts closed, and the module resets the
	// mutation on both ends — reopening always starts clean without a
	// hand-rolled reset here.
	const closeLifecycleOptions = () => setPendingAction(null);

	const isRunning = quadlet.state === "running";
	return (
		<>
			<Tr data-testid="quadlet-row">
				<Td fontFamily="mono">{quadlet.name}</Td>
				<Td fontFamily="mono">{quadlet.resource}</Td>
				<Td>
					<StateBadge state={quadlet.state} />
				</Td>
				<Td>
					<HStack spacing={2}>
						{!isRunning ? (
							<Button
								size="xs"
								variant="outline"
								colorScheme="green"
								aria-label={`Start ${quadlet.name}`}
								onClick={() => startQuadlet.mutate(quadlet.id)}
								isLoading={startQuadlet.isPending}
								isDisabled={lifecyclePending}
								data-testid="start-quadlet-button"
							>
								Start
							</Button>
						) : null}
						{isRunning ? (
							<Button
								size="xs"
								variant="outline"
								aria-label={`Stop ${quadlet.name}`}
								onClick={() => setPendingAction("stop")}
								isDisabled={lifecyclePending}
								data-testid="stop-quadlet-button"
							>
								Stop
							</Button>
						) : null}
						<Button
							size="xs"
							variant="outline"
							aria-label={`Restart ${quadlet.name}`}
							onClick={() => setPendingAction("restart")}
							isDisabled={lifecyclePending}
							data-testid="restart-quadlet-button"
						>
							Restart
						</Button>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Logs ${quadlet.name}`}
							onClick={() => onShowLogs(quadlet)}
						>
							Logs
						</Button>
					</HStack>
				</Td>
				<Td>
					<Button
						size="xs"
						variant="outline"
						aria-label={`Edit ${quadlet.name}`}
						onClick={() => onEdit(quadlet)}
						data-testid="edit-quadlet-button"
					>
						Edit
					</Button>
				</Td>
				<Td>
					<Button
						size="xs"
						colorScheme="red"
						variant="outline"
						aria-label={`Delete ${quadlet.name}`}
						onClick={() => onDelete(quadlet)}
						data-testid="delete-quadlet-button"
					>
						Delete
					</Button>
				</Td>
			</Tr>
			{startQuadlet.error ? (
				<Tr>
					<Td colSpan={6}>
						<ErrorDisplay error={getApiError(startQuadlet.error)} />
					</Td>
				</Tr>
			) : null}
			{openAction ? (
				<LifecycleOptionsModal
					quadlet={quadlet}
					actionLabel={openAction.label}
					lifecycle={openAction.mutation}
					onClose={closeLifecycleOptions}
				/>
			) : null}
		</>
	);
}
