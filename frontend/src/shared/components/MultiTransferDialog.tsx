import { Box, Button, HStack, Text, VStack } from "@chakra-ui/react";
import { Modal } from "@shared/components/Modal";
import type {
	TransferMode,
	TransferState,
} from "@shared/components/TransferDialog";
import { transferErrorMessage } from "@shared/transfers/transferHooks";
import {
	TransferModeRadio,
	type TransferPreviewSlice,
	TransferPreviewStates,
	TransferTargetSelect,
} from "@shared/transfers/transferPresentation";
import type { Host } from "@shared/types/api";
import { type ReactNode, useRef, useState } from "react";

// One item of a batch transfer: the caller's own row object, the selection
// key the list will clear on success, the label rendered under the item's
// block and in the report, that item's target-scoped preview, and its domain
// warnings (the referencing-quadlet list, a stranded-data note, …) rendered
// under its name.
export interface MultiTransferItem<TItem> {
	item: TItem;
	key: string;
	label: string;
	preview: TransferPreviewSlice;
	warnings?: ReactNode;
}

// What one item's run is handed: the batch's single target and mode choice,
// the signal the dialog aborts when it closes mid-run, and the live progress
// line the report row shows while the item is in flight.
export interface MultiTransferRunContext {
	targetId: string;
	targetName: string;
	mode: TransferMode;
	signal: AbortSignal;
	onProgress: (detail: string) => void;
}

// What one item's run reports back: whether it succeeded, and the detail the
// report row shows — "Moved", "Copied (1.2 MB)", or the failure message.
export interface MultiTransferOutcome {
	ok: boolean;
	detail: string;
}

// A report row's lifecycle: not started yet, in flight, then settled.
type ReportStatus = "pending" | "running" | "ok" | "failed";

interface ReportRow {
	key: string;
	label: string;
	detail: string;
	status: ReportStatus;
}

// The batch sibling of TransferDialog (spec 486, ticket #487): one dialog for
// a whole selection, where the target host and the move/copy mode are chosen
// once and every selected item's preview and warnings render under its own
// name. Nothing runs until the user confirms the batch; the confirm then runs
// the per-item requests sequentially in list order, client-side, over the
// existing per-item endpoints — no backend or API change — and the body
// becomes a per-item report. The succeeded keys go back to the list through
// `onFinished` exactly once, so the adapter can drop only those from its
// selection and leave the failures selected for the retry.
//
// The module owns the batch ritual and nothing domain: the target select and
// mode radio reuse the single dialog's testids so the target/mode ritual is
// identical, the per-item preview states are the same three the single
// dialog renders, and the report is the module's own (a batch has no single
// envelope to narrate). The adapter keeps its preview hooks, its mutation or
// stream hook, its warnings, and its outcome mapping.
export function MultiTransferDialog<TItem>({
	host,
	transfer,
	noun,
	pinnedMode,
	items,
	runItem,
	onFinished,
	onClose,
	noTargetsMessage,
	confirmDisabled = false,
	children,
}: {
	host: Host;
	transfer: TransferState;
	// The singular resource noun; the mode radio, confirm, collision, and
	// empty-picker copy derive from it.
	noun: string;
	// A copy-only surface pins the mode: the radio hides and the confirm and
	// title speak Copy vocabulary only.
	pinnedMode?: "copy";
	items: MultiTransferItem<TItem>[];
	runItem: (
		item: TItem,
		context: MultiTransferRunContext,
	) => Promise<MultiTransferOutcome>;
	// Fired exactly once per completed run, with the keys that succeeded. An
	// abandoned run (the dialog closed mid-flight) reports nothing.
	onFinished: (succeededKeys: string[]) => void;
	onClose: () => void;
	noTargetsMessage?: string;
	// Domain refusals beyond the shared set (a differing companion, an
	// unacknowledged stranded-data warning).
	confirmDisabled?: boolean;
	children?: ReactNode;
}) {
	const { targets, targetId, targetName, selectTarget, mode, setMode } =
		transfer;
	// A pinned surface never reads the state's mode: the radio is gone, so
	// the confirm and title speak the pinned vocabulary only.
	const effectiveMode = pinnedMode ?? mode;
	const [rows, setRows] = useState<ReportRow[] | null>(null);
	const abortRef = useRef<AbortController | null>(null);

	const count = items.length;
	const plural = count === 1 ? noun : `${noun}s`;
	// Confirm waits for every preview to settle: one pending, failed, or
	// colliding item blocks the whole batch, because the run is one choice
	// over one target.
	const previewBlocked = items.some(
		(entry) =>
			entry.preview.isPending ||
			entry.preview.isError ||
			!!entry.preview.data?.collision,
	);
	const confirmButtonDisabled =
		targetId === "" || count === 0 || previewBlocked || confirmDisabled;

	// Closing mid-run aborts the in-flight request and closes: the run's
	// remaining items never start, and the list hears nothing back.
	const close = () => {
		abortRef.current?.abort();
		abortRef.current = null;
		onClose();
	};

	const updateRow = (index: number, patch: Partial<ReportRow>) => {
		setRows((current) =>
			current
				? current.map((row, position) =>
						position === index ? { ...row, ...patch } : row,
					)
				: current,
		);
	};

	const confirm = () => {
		const controller = new AbortController();
		abortRef.current = controller;
		setRows(
			items.map((entry) => ({
				key: entry.key,
				label: entry.label,
				detail: "Waiting…",
				status: "pending",
			})),
		);
		void (async () => {
			const succeededKeys: string[] = [];
			// Sequential on purpose: the per-item endpoints are independent,
			// but a batch that fires them in parallel would hammer the source
			// host and make the report's order meaningless.
			for (const [index, entry] of items.entries()) {
				if (controller.signal.aborted) return;
				updateRow(index, { detail: "Running…", status: "running" });
				let outcome: MultiTransferOutcome;
				try {
					outcome = await runItem(entry.item, {
						targetId,
						targetName,
						mode: effectiveMode,
						signal: controller.signal,
						onProgress: (detail) => {
							if (controller.signal.aborted) return;
							updateRow(index, { detail });
						},
					});
				} catch (error) {
					// A thrown request is a failed item, not a failed batch: the
					// report names it and the rest of the run continues.
					outcome = { ok: false, detail: transferErrorMessage(error) };
				}
				if (controller.signal.aborted) return;
				if (outcome.ok) succeededKeys.push(entry.key);
				updateRow(index, {
					detail: outcome.detail,
					status: outcome.ok ? "ok" : "failed",
				});
			}
			abortRef.current = null;
			onFinished(succeededKeys);
		})();
	};

	return (
		<Modal
			isOpen
			onClose={close}
			title={`${pinnedMode ? "Copy" : "Move or copy"} ${count} ${plural} from ${host.name}`}
			footer={
				rows ? (
					<Button
						onClick={close}
						data-testid="close-multi-transfer-report-button"
					>
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={close}>
							Cancel
						</Button>
						<Button
							colorScheme={effectiveMode === "move" ? "red" : "blue"}
							onClick={confirm}
							isDisabled={confirmButtonDisabled}
							data-testid="confirm-multi-transfer-button"
						>
							{effectiveMode === "move" ? "Move" : "Copy"}
						</Button>
					</>
				)
			}
		>
			{rows ? (
				<VStack
					align="flex-start"
					spacing={2}
					data-testid="multi-transfer-report"
				>
					{rows.map((row) => (
						<HStack
							key={row.key}
							spacing={2}
							data-testid="multi-transfer-report-row"
							data-status={row.status}
						>
							<Text fontFamily="mono">{row.label}</Text>
							<Text color={row.status === "failed" ? "red.500" : "gray.500"}>
								{row.detail}
							</Text>
						</HStack>
					))}
				</VStack>
			) : (
				<VStack align="flex-start" spacing={4}>
					<TransferTargetSelect
						targets={targets}
						targetId={targetId}
						onSelect={selectTarget}
						noun={noun}
						noTargetsMessage={noTargetsMessage}
					/>
					<TransferModeRadio
						mode={mode}
						onModeChange={setMode}
						noun={noun}
						pinnedMode={pinnedMode}
					/>
					{items.map((entry) => (
						<Box key={entry.key} w="100%" data-testid="multi-transfer-item">
							<Text fontWeight="semibold" fontFamily="mono">
								{entry.label}
							</Text>
							<TransferPreviewStates
								preview={entry.preview}
								noun={noun}
								itemName={entry.label}
								targetName={targetName}
								testIds={{
									pending: "multi-transfer-item-pending",
									error: "multi-transfer-item-error",
									collision: "multi-transfer-item-collision",
								}}
							/>
							{entry.warnings}
						</Box>
					))}
					{children}
				</VStack>
			)}
		</Modal>
	);
}
