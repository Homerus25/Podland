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
import type {
	useRestartQuadlet,
	useStartQuadlet,
	useStopQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import type {
	QuadletRef,
	QuadletSummary,
} from "@features/quadlets/services/quadletsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { StateBadge } from "@shared/components/StateBadge";
import { identityKey, rowScoped } from "@shared/queries/rowScopedMutation";
import { getApiError } from "@shared/utils/api";
import { Fragment, useState } from "react";

// Stop and restart share the backend's option set (timeout only — there is
// no force param, issue 205), so one modal serves both: the caller names the
// action and owns the mutation. Defaults mirror the backend's own (timeout
// unset and therefore omitted from the request), so confirming untouched
// sends no query parameters. The gray confirm opts out of the preset's
// destructive red — stopping a unit is routine, not danger — and the footer
// plumbing (reset lifecycle, close-on-success, pending wiring, error region
// above the body) is the module's.
//
// The list renders this, not the row: a plain row function cannot own the
// open/closed state, so the list holds which action is open and mounts the
// modal from there. The detail page mounts it too (issue #472), with the
// same shape.
export function LifecycleOptionsModal({
	quadlet,
	actionLabel,
	lifecycle,
	onClose,
}: {
	quadlet: QuadletRef;
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

// The lifecycle mutations the list owns, one instance per action, handed to
// every row. A plain row function cannot own hooks, so the list keeps them
// and each row scopes its own pending state off the mutation's variables
// through the shared projection. Start takes the quadlet id as its
// variables (the shared identity key); stop and restart take the
// options object, so their key extractor reads the quadletId field.
export type QuadletLifecycleMutations = {
	start: ReturnType<typeof useStartQuadlet>;
	stop: ReturnType<typeof useStopQuadlet>;
	restart: ReturnType<typeof useRestartQuadlet>;
};

const quadletOptionsKey = (variables: { quadletId: string }) =>
	variables.quadletId;

// The row is a plain function returning a Fragment of <Tr>s, not a
// component: the list's cell-cloning walk (mobile pinning and the selection
// checkbox column) reaches a row's cells only when `renderRow` hands back a
// `Tr` or a `Fragment` of `Tr`s — a component element is opaque to it, so a
// row wrapped in one silently loses both. The key rides the returned
// Fragment, which is the element the list's map places.
//
// Any in-flight lifecycle action takes the row's other lifecycle buttons with
// it, so slow systemd operations don't invite double clicks; the scoping by
// variables keeps other rows usable. Stop and restart run through the
// options modal, which the list mounts — the row only names the action.
//
// The lifecycle buttons are write-gated (issue #484): below the
// manage-quadlets capability the row hides them and leaves Logs — the
// list's hide-and-hint pattern, whose hint is ResourceList's own (the
// module swaps the toolbar for it), so the row renders no hint itself.
//
// The row navigates to the quadlet's detail page on click (issue #472, the
// volumes list's pattern), so every action button stops the click from
// reaching the row. The row carries no destructive button (spec 461, ticket
// #470): single-item removal lives on the detail page's Remove button, and
// the list keeps its bulk "Remove selected".
export function renderQuadletRow({
	quadlet,
	canManage,
	lifecycle,
	onEdit,
	onMove,
	onShowLogs,
	onLifecycle,
	onOpen,
}: {
	quadlet: QuadletSummary;
	canManage: boolean;
	lifecycle: QuadletLifecycleMutations;
	onEdit: (quadlet: QuadletSummary) => void;
	onMove: (quadlet: QuadletSummary) => void;
	onShowLogs: (quadlet: QuadletSummary) => void;
	onLifecycle: (quadlet: QuadletSummary, action: "stop" | "restart") => void;
	onOpen: (quadlet: QuadletSummary) => void;
}) {
	const startScoped = rowScoped(lifecycle.start, quadlet.id, identityKey);
	const stopScoped = rowScoped(lifecycle.stop, quadlet.id, quadletOptionsKey);
	const restartScoped = rowScoped(
		lifecycle.restart,
		quadlet.id,
		quadletOptionsKey,
	);
	const startPending = startScoped.isPending;
	const stopPending = stopScoped.isPending;
	const restartPending = restartScoped.isPending;
	const lifecyclePending = startPending || stopPending || restartPending;
	// Only a failed start shows under the row; stop and restart report inside
	// their still-open options modal.
	const rowError = startScoped.error;

	const isRunning = quadlet.state === "running";
	return (
		<Fragment key={quadlet.id}>
			<Tr
				data-testid="quadlet-row"
				cursor="pointer"
				onClick={() => onOpen(quadlet)}
			>
				<Td fontFamily="mono">{quadlet.name}</Td>
				<Td fontFamily="mono">{quadlet.resource}</Td>
				<Td>
					<StateBadge state={quadlet.state} />
				</Td>
				<Td>
					<HStack spacing={2}>
						{canManage ? (
							<>
								{!isRunning ? (
									<Button
										size="xs"
										variant="outline"
										colorScheme="green"
										aria-label={`Start ${quadlet.name}`}
										onClick={(event) => {
											event.stopPropagation();
											lifecycle.start.mutate(quadlet.id);
										}}
										isLoading={startPending}
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
										onClick={(event) => {
											event.stopPropagation();
											onLifecycle(quadlet, "stop");
										}}
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
									onClick={(event) => {
										event.stopPropagation();
										onLifecycle(quadlet, "restart");
									}}
									isDisabled={lifecyclePending}
									data-testid="restart-quadlet-button"
								>
									Restart
								</Button>
							</>
						) : null}
						<Button
							size="xs"
							variant="outline"
							aria-label={`Logs ${quadlet.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onShowLogs(quadlet);
							}}
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
						onClick={(event) => {
							event.stopPropagation();
							onEdit(quadlet);
						}}
						data-testid="edit-quadlet-button"
					>
						Edit
					</Button>
				</Td>
				{canManage ? (
					<Td>
						<Button
							size="xs"
							variant="outline"
							aria-label={`Move or copy ${quadlet.name}`}
							onClick={(event) => {
								event.stopPropagation();
								onMove(quadlet);
							}}
							data-testid="move-quadlet-button"
						>
							Move/Copy
						</Button>
					</Td>
				) : null}
			</Tr>
			{rowError ? (
				<Tr>
					<Td colSpan={canManage ? 6 : 5}>
						<ErrorDisplay error={getApiError(rowError)} />
					</Td>
				</Tr>
			) : null}
		</Fragment>
	);
}
