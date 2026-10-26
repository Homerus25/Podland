import {
	Box,
	Button,
	FormControl,
	FormLabel,
	HStack,
	Radio,
	RadioGroup,
	Select,
	Text,
	VStack,
} from "@chakra-ui/react";
import { useMachines } from "@features/machines/hooks/useMachines";
import { type Capability, can } from "@features/machines/permissions";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import type { ApiError, Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { type ReactNode, useEffect, useState } from "react";

// The mode vocabulary every cross-machine transfer shares (the backend's
// MoveMode* unions): "copy" installs on the target, "move" also removes the
// source. The feature DTOs (QuadletMoveMode, VolumeMoveMode, SecretMoveMode)
// are the same union; the preset speaks it once.
export type TransferMode = "copy" | "move";

// The structural slice of a target-scoped move-preview query the preset
// drives. The feature keeps its own preview hook and query key and hands
// over the result; the preset reads only the states every transfer shares —
// pending, failure, and the collision flag — never the domain fields.
export interface TransferPreviewSlice {
	isPending: boolean;
	isError: boolean;
	error: unknown;
	data: { collision: boolean } | undefined;
}

// The skeleton's state, owned by useTransferState and handed to the
// component whole: the writable targets, the selected one (defaulting to
// the first), its display name, and the mode. A pinned (copy-only)
// surface never reads mode/setMode; the shared shape keeps them for the
// mode-choosing surfaces.
export interface TransferState {
	targets: Machine[];
	targetId: string;
	targetName: string;
	selectTarget: (targetId: string) => void;
	mode: TransferMode;
	setMode: (mode: TransferMode) => void;
}

// The state half of the transfer-dialog preset: the capability-filtered
// target list (source excluded — the backend refuses below-write targets
// with 403, so the picker never offers one) and the two states every
// transfer dialog carries. The select starts on the first writable machine,
// so the preview loads immediately; empty means no writable target exists.
// The mode defaults to Move. The adapter calls this first: its own
// target-scoped preview hook needs targetId, and its domain body and report
// need mode and targetName.
export function useTransferState(
	machine: Machine,
	capability: Capability,
): TransferState {
	const { data: machines } = useMachines();
	const targets = (machines ?? []).filter(
		(candidate) => candidate.id !== machine.id && can(candidate, capability),
	);
	const [selectedTargetId, setSelectedTargetId] = useState("");
	const targetId = selectedTargetId || targets[0]?.id || "";
	const targetName =
		targets.find((candidate) => candidate.id === targetId)?.name ?? targetId;
	const [mode, setMode] = useState<TransferMode>("move");
	return {
		targets,
		targetId,
		targetName,
		selectTarget: setSelectedTargetId,
		mode,
		setMode,
	};
}

// One owner of the stay-open transfer dialog skeleton the three
// cross-machine move surfaces (quadlet, volume, secret) hand-rolled: the
// capability-filtered target select, the mode radio defaulting to Move, the
// target-scoped preview's pending/error/collision states, the
// referencing-quadlet warning, the confirm/report footer swap, and the
// in-modal error region with its hint seam. Like PruneReportDialog it is
// standalone from MutationDialog because the two disagree on the headline
// invariant: a transfer's success behavior is mixed — the report narrates
// in place and a "partial" result must keep the dialog open naming the
// both-machines state — which the core's unconditional close-on-success
// cannot express. The caller renders the dialog conditionally, so mount is
// open and unmount is close.
//
// Everything per-feature arrives as data: the noun (the mode radio, confirm
// button, collision, and referencing copy derive from it), the item name,
// the preview result, the transfer's pending/error state, the stay-open
// report node, and — through the two body slots — the domain warnings and
// options. The `warnings` slot renders between the collision block and the
// referencing warning, `children` after it, so an adapter with referencing
// quadlets keeps its own warning order. The module never touches
// react-query: the feature keeps its preview hook and its mutation or
// stream hook as thin adapters over the shared transfer-hooks module
// (`@shared/transfers`, issue 273), which owns the preview ritual, the
// both-machines invalidation, and the move-stream lifecycle; the feature
// keeps its domain — the key constructor and fetcher, the event
// vocabulary, shape guard, progress fold, and URL builder.
export function TransferDialog({
	machine,
	transfer,
	preview,
	pinnedMode,
	noun,
	itemName,
	noTargetsMessage,
	referencingQuadlets = [],
	isPending,
	confirmDisabled = false,
	error,
	renderHints,
	report,
	reset,
	warnings,
	children,
	onConfirm,
	onClose,
}: {
	machine: Machine;
	transfer: TransferState;
	preview: TransferPreviewSlice;
	// A copy-only surface (the image copy, issue 376) pins the mode:
	// "copy" is the only pinnable mode — no surface pins a move. The
	// radio hides and the confirm and title speak Copy vocabulary only.
	// The three mode-choosing surfaces pass nothing and render
	// byte-identically.
	pinnedMode?: "copy";
	// The singular resource noun; the mode radio, confirm button, collision,
	// and referencing copy derive from it.
	noun: string;
	itemName: string;
	// The empty-picker copy; defaults to the manage-{noun}s phrasing. The
	// quadlet surface keeps its own "you can write to" wording.
	noTargetsMessage?: string;
	// Quadlets on the source that reference the transferred item: rendered
	// as a warning, never a block, with the dangling note in move mode only.
	referencingQuadlets?: ReadonlyArray<{ name: string }>;
	// The transfer's in-flight state: feeds the confirm's loading state and
	// its disabled set alike.
	isPending: boolean;
	// Domain refusals beyond the shared set (a differing companion, an
	// unacknowledged stranded-data warning).
	confirmDisabled?: boolean;
	error: ApiError | null;
	// Where error-code-to-hint mapping lives, the MutationDialog
	// renderError precedent: the region renders the error display, the seam
	// renders the hint(s) under it.
	renderHints?: (error: ApiError) => ReactNode;
	// The stay-open report: non-null flips the footer to Close and replaces
	// the form body. A partial result is a report like any other — the
	// adapter decides what narrates.
	report: ReactNode | null;
	// The reset invariant for a caller-owned mutation that outlives the
	// dialog (the quadlet move mutation lives in the list): reset() runs on
	// mount and unmount so reopening always starts clean. Component-local
	// hooks die with the dialog and pass nothing. The effect keys on the
	// reference, so pass a stable one (a react-query mutation's reset).
	reset?: () => void;
	warnings?: ReactNode;
	children?: ReactNode;
	onConfirm: () => void;
	onClose: () => void;
}) {
	const { targets, targetId, targetName, selectTarget, mode, setMode } =
		transfer;
	// A pinned surface never reads the state's mode: the radio is gone, so
	// the confirm and title speak the pinned vocabulary only.
	const effectiveMode = pinnedMode ?? mode;

	useEffect(() => {
		if (!reset) return;
		reset();
		return () => reset();
	}, [reset]);

	const confirmButtonDisabled =
		targetId === "" ||
		isPending ||
		preview.isPending ||
		preview.isError ||
		!!preview.data?.collision ||
		confirmDisabled;

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={pinnedMode ? `Copy ${itemName}` : `Move or copy ${itemName}`}
			footer={
				report ? (
					<Button onClick={onClose} data-testid="close-move-report-button">
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={onClose}>
							Cancel
						</Button>
						<Button
							colorScheme={effectiveMode === "move" ? "red" : "blue"}
							onClick={onConfirm}
							isDisabled={confirmButtonDisabled}
							isLoading={isPending}
							data-testid="confirm-move-button"
						>
							{effectiveMode === "move" ? `Move ${noun}` : `Copy ${noun}`}
						</Button>
					</>
				)
			}
		>
			{error ? (
				<Box mt={2}>
					<ErrorDisplay error={error} data-testid="mutation-dialog-error" />
					{renderHints ? renderHints(error) : null}
				</Box>
			) : null}
			{report ?? (
				<VStack align="flex-start" spacing={4}>
					<FormControl isRequired>
						<FormLabel htmlFor="move-target">Target machine</FormLabel>
						{targets.length > 0 ? (
							<Select
								id="move-target"
								value={targetId}
								onChange={(event) => selectTarget(event.target.value)}
								data-testid="move-target-select"
							>
								{targets.map((candidate) => (
									<option key={candidate.id} value={candidate.id}>
										{candidate.name}
									</option>
								))}
							</Select>
						) : (
							<Text color="gray.500" data-testid="move-no-targets">
								{noTargetsMessage ??
									`No other machine you can manage ${noun}s on.`}
							</Text>
						)}
					</FormControl>
					{pinnedMode ? null : (
						<FormControl>
							<FormLabel>Mode</FormLabel>
							<RadioGroup
								value={mode}
								onChange={(next) => setMode(next as TransferMode)}
								data-testid="move-mode-radio"
							>
								<HStack spacing={4}>
									<Radio value="move" data-testid="move-mode-move">
										Move (remove from this machine)
									</Radio>
									<Radio value="copy" data-testid="move-mode-copy">
										Copy (keep this machine&apos;s {noun})
									</Radio>
								</HStack>
							</RadioGroup>
						</FormControl>
					)}
					{preview.isPending ? (
						<Text color="gray.500" data-testid="move-preview-pending">
							Checking the target machine…
						</Text>
					) : null}
					{preview.isError ? (
						<ErrorDisplay
							error={getApiError(preview.error)}
							data-testid="move-preview-error"
						/>
					) : null}
					{preview.data?.collision ? (
						<Text color="red.500" data-testid="move-collision-warning">
							A {noun} named “{itemName}” already exists on {targetName}. Choose
							another target or remove it there first.
						</Text>
					) : null}
					{warnings}
					{referencingQuadlets.length > 0 ? (
						<Box data-testid="move-referencing-warning">
							<Text>
								Quadlets on this machine reference this {noun}. They are not
								transferred and keep pointing at {machine.name}:
							</Text>
							<VStack align="flex-start" mt={2} spacing={1}>
								{referencingQuadlets.map((quadlet) => (
									<HStack
										key={quadlet.name}
										spacing={2}
										data-testid="move-referencing-row"
									>
										<Text fontFamily="mono">{quadlet.name}</Text>
									</HStack>
								))}
							</VStack>
							{effectiveMode === "move" ? (
								<Text mt={2} color="orange.500">
									After a move these references dangle — the {noun} they name is
									gone from this machine.
								</Text>
							) : null}
						</Box>
					) : null}
					{children}
				</VStack>
			)}
		</Modal>
	);
}
