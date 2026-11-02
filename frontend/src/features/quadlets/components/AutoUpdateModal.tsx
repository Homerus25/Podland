import { CheckCircleIcon, TimeIcon, WarningIcon } from "@chakra-ui/icons";
import {
	Badge,
	Box,
	Button,
	Collapse,
	Heading,
	HStack,
	List,
	ListIcon,
	ListItem,
	Spinner,
	Stack,
	Text,
} from "@chakra-ui/react";
import type {
	AutoUpdateItem,
	AutoUpdateLine,
	AutoUpdateRow,
	UseQuadletAutoUpdateResult,
} from "@features/quadlets/hooks/useQuadletAutoUpdate";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { useEffect, useState } from "react";

// How one report-resolved row renders, keyed by the backend's
// AutoUpdateItem outcome vocabulary ("updated", "failed", "pending",
// "up-to-date"). A pending line in a check window carries an inline Update
// button that runs a real auto-update filtered to the line's unit. The
// button renders only when the modal was opened as a check and the line
// names a unit: a pending line without a unit renders no inline button,
// because there is no unit to filter to. The button needs no disabled state
// of its own: while any run is in progress the row list shows the new
// run's live rows (per-service buttons included are gone with the old
// rows), and the footer Update button stays rendered and disabled.
function ReportItem({
	item,
	onUpdateUnit,
}: {
	item: AutoUpdateItem;
	onUpdateUnit: ((unit: string) => void) | null;
}) {
	const testId = `auto-update-item-${item.updated}-${item.containerName}`;
	if (item.updated === "failed") {
		const error = item.error ?? "unknown error";
		return (
			// The inline error is truncated to one line; the row's title
			// carries the full text.
			<ListItem
				data-testid={testId}
				color="red.300"
				title={error}
				noOfLines={1}
			>
				<ListIcon as={WarningIcon} color="red.400" />
				<strong>{item.containerName}</strong> ({item.imageName}) failed: {error}
			</ListItem>
		);
	}
	if (item.updated === "pending") {
		const unit = item.unit;
		return (
			<ListItem data-testid={testId} color="orange.300">
				<ListIcon as={TimeIcon} color="orange.400" />
				<strong>{item.containerName}</strong> has a pending update for{" "}
				{item.imageName}
				{onUpdateUnit && unit ? (
					<Button
						size="xs"
						ml={2}
						colorScheme="orange"
						onClick={() => onUpdateUnit(unit)}
						data-testid={`auto-update-item-update-${item.containerName}`}
					>
						Update
					</Button>
				) : null}
			</ListItem>
		);
	}
	if (item.updated === "up-to-date") {
		return (
			<ListItem data-testid={testId} color="gray.400">
				<ListIcon as={CheckCircleIcon} color="gray.500" />
				<strong>{item.containerName}</strong> ({item.imageName}) is up to date
			</ListItem>
		);
	}
	return (
		<ListItem data-testid={testId} color="green.300">
			<ListIcon as={CheckCircleIcon} color="green.400" />
			<strong>{item.containerName}</strong> updated to {item.imageName}
		</ListItem>
	);
}

// One unresolved live row. Waiting is passive by design — plain, muted, no
// spinner — because nothing has been observed for the unit yet; pulling is
// the one observed activity, so it alone gets a spinner. Both stay honest
// to what the stream reported.
function LiveRow({ row }: { row: AutoUpdateRow }) {
	if (row.state === "pulling") {
		return (
			<ListItem data-testid={`auto-update-row-pulling-${row.unit}`}>
				<HStack spacing={2} alignItems="center">
					<Spinner
						size="sm"
						data-testid={`auto-update-row-spinner-${row.unit}`}
					/>
					<Text color="gray.300">
						{row.unit}
						{row.image ? ` (${row.image})` : ""}
					</Text>
				</HStack>
			</ListItem>
		);
	}
	return (
		<ListItem
			data-testid={`auto-update-row-waiting-${row.unit}`}
			color="gray.500"
		>
			{row.unit}
			{row.image ? ` (${row.image})` : ""}
		</ListItem>
	);
}

// The raw podman output, one line per streamed entry.
function RawOutputLines({ lines }: { lines: AutoUpdateLine[] }) {
	return (
		<>
			{lines.map((entry, index) => (
				// biome-ignore lint/suspicious/noArrayIndexKey: append-only output lines have no stable id; index is their identity
				<Text key={index} as="span" display="block" whiteSpace="pre-wrap">
					{entry.line}
				</Text>
			))}
		</>
	);
}

// Live view of one auto-update run. The primary view is the per-quadlet
// status list, folded hook-side from the stream's plan, pulling, and
// report events: planned units render up front as waiting rows, a pulling
// event flips its row to a spinner and names the unit in the in-progress
// header, and the completing report resolves every row to its outcome
// badge. The raw podman output is a collapsible diagnostic fallback. Dry
// runs say so outright; a terminal error event is a connection problem
// (the run never produced a report) and is shown apart from per-container
// failures. A modal opened as a check (a dry run) offers a footer Update
// button that turns the same window into a real run, and an inline Update
// button on every pending report line that names a unit, running a real
// auto-update filtered to that line's unit; a modal opened as a real run
// from the toolbar never renders either. A filtered run names its unit in
// the title, while it runs and after it completes.
export function AutoUpdateModal({
	onClose,
	dryRun,
	openedAsCheck,
	autoUpdate,
}: {
	onClose: () => void;
	dryRun: boolean;
	openedAsCheck: boolean;
	autoUpdate: UseQuadletAutoUpdateResult;
}) {
	const title = autoUpdate.filter
		? `Update ${autoUpdate.filter}`
		: dryRun
			? "Check for updates"
			: "Update all";
	const pendingCount = autoUpdate.report
		? autoUpdate.report.containers.filter((item) => item.updated === "pending")
				.length
		: 0;
	// Enabled iff a completed report has at least one pending container; a
	// running check, a connection error, a missing or unparsable report, and
	// zero pending containers all keep it disabled (it stays rendered, so
	// there is no layout jump between runs).
	const updateDisabled =
		autoUpdate.isRunning ||
		autoUpdate.connectionError !== null ||
		!autoUpdate.report ||
		autoUpdate.report.overall === "unparsable-report" ||
		pendingCount === 0;
	// Per-service updates exist only in a check window: each is a real run
	// filtered to the line's unit. Null when the modal was opened as a real
	// run from the toolbar, so no report line renders an inline button.
	const onUpdateUnit = openedAsCheck
		? (unit: string) => autoUpdate.run(false, unit)
		: null;
	const reportHasFailures =
		autoUpdate.report?.containers.some((item) => item.updated === "failed") ??
		false;
	// The raw podman output is a diagnostic fallback, not the primary view:
	// collapsed by default, expanded on demand, and expanded automatically
	// when the report contains a failure so diagnostic detail is not missed.
	// A new run re-collapses it.
	const [outputOpen, setOutputOpen] = useState(false);
	useEffect(() => {
		if (reportHasFailures) setOutputOpen(true);
	}, [reportHasFailures]);
	useEffect(() => {
		if (autoUpdate.isRunning) setOutputOpen(false);
	}, [autoUpdate.isRunning]);
	return (
		<Modal
			isOpen
			onClose={onClose}
			title={title}
			footer={
				<>
					{openedAsCheck ? (
						<Button
							colorScheme="orange"
							isDisabled={updateDisabled}
							onClick={() => autoUpdate.run(false)}
							data-testid="auto-update-button"
						>
							Update ({pendingCount})
						</Button>
					) : null}
					<Button
						variant="ghost"
						onClick={onClose}
						data-testid="auto-update-close-button"
					>
						{autoUpdate.isRunning ? "Cancel" : "Close"}
					</Button>
				</>
			}
		>
			<HStack>
				<Heading size="sm">{title}</Heading>
				{dryRun ? (
					<Badge colorScheme="purple" data-testid="auto-update-dry-run-badge">
						Dry run
					</Badge>
				) : (
					<Badge colorScheme="orange" data-testid="auto-update-real-run-badge">
						Applying updates
					</Badge>
				)}
			</HStack>
			{autoUpdate.isRunning ? (
				<HStack mt={3} data-testid="auto-update-in-progress">
					<Spinner size="sm" />
					<Text>
						{autoUpdate.pullingUnit
							? `Pulling ${autoUpdate.pullingUnit}…`
							: "Running…"}
					</Text>
				</HStack>
			) : null}
			{autoUpdate.rows.length > 0 ? (
				<Stack spacing={2} mt={3}>
					{!autoUpdate.isRunning && reportHasFailures ? (
						<Text data-testid="auto-update-overall" color="red.300">
							{autoUpdate.report?.overall}
						</Text>
					) : null}
					<List spacing={1}>
						{autoUpdate.rows.map((row, index) =>
							row.item ? (
								<ReportItem
									key={`${row.item.containerName}-${row.item.updated}`}
									item={row.item}
									onUpdateUnit={onUpdateUnit}
								/>
							) : (
								<LiveRow key={row.unit ?? `row-${index}`} row={row} />
							),
						)}
					</List>
				</Stack>
			) : null}
			{autoUpdate.lines.length > 0 ? (
				<Stack mt={3} spacing={1}>
					<Button
						size="xs"
						variant="ghost"
						alignSelf="flex-start"
						aria-expanded={outputOpen}
						onClick={() => setOutputOpen((open) => !open)}
						data-testid="auto-update-output-toggle"
					>
						Raw output
					</Button>
					<Collapse in={outputOpen}>
						<Box
							p={2}
							bg="blackAlpha.400"
							borderRadius="md"
							fontFamily="mono"
							fontSize="xs"
							maxH="200px"
							overflowY="auto"
							data-testid="auto-update-output"
						>
							<RawOutputLines lines={autoUpdate.lines} />
						</Box>
					</Collapse>
				</Stack>
			) : null}
			{autoUpdate.connectionError ? (
				<Box mt={3} data-testid="auto-update-connection-error">
					<ErrorDisplay error={autoUpdate.connectionError} />
				</Box>
			) : null}
			{!autoUpdate.isRunning &&
			autoUpdate.report &&
			autoUpdate.rows.length === 0 ? (
				<Text mt={3}>No auto-update containers found on this machine.</Text>
			) : null}
			{!autoUpdate.isRunning &&
			!autoUpdate.report &&
			!autoUpdate.connectionError ? (
				<Text mt={3}>Done.</Text>
			) : null}
		</Modal>
	);
}
