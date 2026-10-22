import { CheckCircleIcon, WarningIcon } from "@chakra-ui/icons";
import {
	Badge,
	Box,
	Button,
	HStack,
	Input,
	List,
	ListIcon,
	ListItem,
	Stack,
	Text,
	Textarea,
} from "@chakra-ui/react";
import {
	useConvertCompose,
	useImportQuadlets,
} from "@features/quadlets/hooks/useQuadlets";
import type {
	QuadletImportEntry,
	QuadletImportError,
	QuadletImportResult,
} from "@features/quadlets/services/quadletsApi";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { useState } from "react";

// The three phases of the import-compose flow (spec #336): input (paste or
// pick compose content), preview (review the generated quadlets read-only,
// abort before anything is installed), report (the import endpoint's
// imported/failed answer per file). The flow does not fit the
// MutationDialog presets — two chained mutations with a read-only middle
// phase — so the modal is feature-owned, like AutoUpdateModal. The list
// renders it conditionally (mount = open, unmount = close), so reopening
// always starts clean: no leaked previous conversion.
type ImportPhase = "input" | "preview" | "report";

// Input phase: the textarea is the one source of truth — the file picker
// loads the chosen file's content into it, so what the user sees is exactly
// what gets converted. The convert request is always a JSON body, never
// multipart.
function InputPhase({
	content,
	onContentChange,
	onFileChosen,
}: {
	content: string;
	onContentChange: (content: string) => void;
	onFileChosen: (file: File) => void;
}) {
	return (
		<Stack spacing={3}>
			<Textarea
				aria-label="Compose content"
				data-testid="compose-content-input"
				fontFamily="mono"
				rows={10}
				placeholder={"services:\n  web:\n    image: nginx:1.27"}
				value={content}
				onChange={(event) => onContentChange(event.target.value)}
			/>
			<Input
				type="file"
				data-testid="compose-file-input"
				onChange={(event) => {
					const file = event.target.files?.[0];
					if (file) onFileChosen(file);
				}}
			/>
		</Stack>
	);
}

// Preview phase: the generated quadlet files render read-only (name plus
// content), with the hint that relative bind paths resolve against the
// quadlet directory on the target machine.
function PreviewPhase({ quadlets }: { quadlets: QuadletImportEntry[] }) {
	return (
		<Stack spacing={3}>
			<Text data-testid="import-relative-path-hint" color="gray.400">
				Relative bind paths in the generated quadlets resolve against the
				quadlet directory on the target machine.
			</Text>
			{quadlets.map((entry) => (
				<Box
					key={`${entry.name}.${entry.resource}`}
					data-testid={`import-preview-file-${entry.name}`}
					p={2}
					bg="blackAlpha.400"
					borderRadius="md"
				>
					<HStack mb={1} spacing={2}>
						<Text fontWeight="bold">{entry.name}</Text>
						<Badge>{entry.resource}</Badge>
					</HStack>
					<Box
						as="pre"
						fontFamily="mono"
						fontSize="xs"
						whiteSpace="pre-wrap"
						maxH="200px"
						overflowY="auto"
					>
						{entry.content}
					</Box>
				</Box>
			))}
		</Stack>
	);
}

// Report phase: the import endpoint's imported/failed answer, per file.
function ReportPhase({
	imported,
	failed,
}: {
	imported: QuadletImportResult[];
	failed: QuadletImportError[];
}) {
	return (
		<List spacing={1}>
			{imported.map((item) => (
				<ListItem
					key={item.id}
					data-testid={`import-report-imported-${item.name}`}
					color="green.300"
				>
					<ListIcon as={CheckCircleIcon} color="green.400" />
					<strong>{item.name}</strong> imported to {item.path} ({item.state})
				</ListItem>
			))}
			{failed.map((item) => (
				<ListItem
					key={item.name}
					data-testid={`import-report-failed-${item.name}`}
					color="red.300"
				>
					<ListIcon as={WarningIcon} color="red.400" />
					<strong>{item.name}</strong> failed: {item.error}
				</ListItem>
			))}
		</List>
	);
}

export function ImportComposeModal({
	machineId,
	onClose,
}: {
	machineId: string;
	onClose: () => void;
}) {
	const [phase, setPhase] = useState<ImportPhase>("input");
	const [content, setContent] = useState("");
	const [quadlets, setQuadlets] = useState<QuadletImportEntry[]>([]);
	const convert = useConvertCompose();
	const install = useImportQuadlets(machineId);

	// The file picker loads the chosen file's text into the textarea — one
	// source of truth, so what the user sees is exactly what gets converted.
	const onFileChosen = (file: File) => {
		void file.text().then(setContent);
	};

	const runConvert = () => {
		convert.mutate(content, {
			onSuccess: (data) => {
				setQuadlets(data.quadlets);
				setPhase("preview");
			},
		});
	};

	// The converted entries forward unchanged to the machine-scoped import
	// endpoint — the convert response's shape is the import request's shape.
	const runInstall = () => {
		install.mutate(
			{ quadlets },
			{
				onSuccess: () => setPhase("report"),
			},
		);
	};

	return (
		<Modal
			isOpen
			onClose={onClose}
			title="Import compose file"
			footer={
				<>
					{phase === "input" ? (
						<Button
							colorScheme="blue"
							isDisabled={!content.trim()}
							isLoading={convert.isPending}
							onClick={runConvert}
							data-testid="convert-compose-button"
						>
							Convert
						</Button>
					) : null}
					{phase === "preview" ? (
						<Button
							colorScheme="blue"
							isLoading={install.isPending}
							onClick={runInstall}
							data-testid="install-quadlets-button"
						>
							Install
						</Button>
					) : null}
					{phase === "report" ? (
						<Button
							colorScheme="blue"
							onClick={onClose}
							data-testid="import-compose-close-button"
						>
							Close
						</Button>
					) : null}
					{phase !== "report" ? (
						<Button
							variant="ghost"
							onClick={onClose}
							data-testid="import-compose-cancel-button"
						>
							Cancel
						</Button>
					) : null}
				</>
			}
		>
			{phase === "input" ? (
				<InputPhase
					content={content}
					onContentChange={setContent}
					onFileChosen={onFileChosen}
				/>
			) : null}
			{phase === "preview" ? <PreviewPhase quadlets={quadlets} /> : null}
			{phase === "report" && install.data ? (
				<ReportPhase
					imported={install.data.imported}
					failed={install.data.failed}
				/>
			) : null}
			{phase === "input" && convert.error ? (
				<Box mt={3} data-testid="import-convert-error">
					<ErrorDisplay error={getApiError(convert.error)} />
				</Box>
			) : null}
			{phase === "preview" && install.error ? (
				<Box mt={3} data-testid="import-install-error">
					<ErrorDisplay error={getApiError(install.error)} />
				</Box>
			) : null}
		</Modal>
	);
}
