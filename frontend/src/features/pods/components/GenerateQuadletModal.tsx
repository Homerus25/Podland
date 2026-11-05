import {
	Badge,
	Box,
	Button,
	HStack,
	Spinner,
	Stack,
	Text,
} from "@chakra-ui/react";
import {
	useCreateQuadlet,
	useGenerateQuadlet,
} from "@features/quadlets/hooks/useQuadlets";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { Modal } from "@shared/components/Modal";
import { getApiError } from "@shared/utils/api";
import { useEffect } from "react";

// The generate-quadlet flow (spec #402 ticket #406) is a multi-phase
// chained flow no MutationDialog preset fits — a read-only generate whose
// answer is offered back through a separate save — so the modal is
// feature-owned over the shared Modal shell, like ImportComposeModal. The
// detail page renders it conditionally (mount = open, unmount = close),
// so reopening always starts clean: no leaked previous generation.
export function GenerateQuadletModal({
	hostId,
	podId,
	podName,
	canSave,
	onClose,
}: {
	hostId: string;
	podId: string;
	podName: string;
	// Save posts to the write-gated quadlet create endpoint, so the page
	// hides it below the manage-quadlets capability; generate and copy
	// stay read-level.
	canSave: boolean;
	onClose: () => void;
}) {
	const generate = useGenerateQuadlet(hostId);
	const save = useCreateQuadlet(hostId);
	const { mutate: runGenerate } = generate;

	// Mount = open: generation starts once per open, and the mutation's
	// pending/error/success states narrate the run. mutate is stable, so
	// the effect keys on the pod only.
	useEffect(() => {
		runGenerate({ resourceId: podId, resourceType: "pod" });
	}, [runGenerate, podId]);

	const generated = generate.data;

	// The generate answer forwards to the create endpoint verbatim — the
	// pod's name, its resource type, and the generated content — so the
	// saved file appears in the host's quadlets list (the create
	// mutation invalidates it).
	const runSave = () => {
		if (!generated) return;
		save.mutate({
			name: generated.name,
			resource: generated.resource,
			content: generated.content,
		});
	};

	return (
		<Modal
			isOpen
			onClose={onClose}
			title={`Generate quadlet — ${podName}`}
			footer={
				save.isSuccess ? (
					<Button
						variant="ghost"
						onClick={onClose}
						data-testid="generate-quadlet-close-button"
					>
						Close
					</Button>
				) : (
					<>
						{generated ? (
							<Button
								variant="outline"
								onClick={() => navigator.clipboard.writeText(generated.content)}
								data-testid="copy-generated-quadlet-button"
							>
								Copy
							</Button>
						) : null}
						{generated && canSave ? (
							<Button
								colorScheme="blue"
								isLoading={save.isPending}
								onClick={runSave}
								data-testid="save-generated-quadlet-button"
							>
								Save to host
							</Button>
						) : null}
						<Button
							variant="ghost"
							onClick={onClose}
							data-testid="generate-quadlet-close-button"
						>
							Close
						</Button>
					</>
				)
			}
		>
			{generate.isPending ? (
				<HStack data-testid="generate-quadlet-pending">
					<Spinner size="sm" />
					<Text>Generating…</Text>
				</HStack>
			) : null}
			{generate.error ? (
				<Box mt={3} data-testid="generate-quadlet-error">
					<ErrorDisplay error={getApiError(generate.error)} />
				</Box>
			) : null}
			{generated ? (
				<Stack spacing={3}>
					<HStack spacing={2}>
						<Text fontWeight="bold">{generated.name}</Text>
						<Badge>{generated.resource}</Badge>
					</HStack>
					<Box
						as="pre"
						fontFamily="mono"
						fontSize="xs"
						whiteSpace="pre-wrap"
						maxH="300px"
						overflowY="auto"
						data-testid="generated-quadlet-content"
					>
						{generated.content}
					</Box>
					{save.isSuccess ? (
						<Text data-testid="generated-quadlet-saved" color="green.300">
							Saved — the quadlet now appears in the host's Quadlets tab.
						</Text>
					) : null}
					{save.error ? (
						<Box data-testid="generate-quadlet-save-error">
							<ErrorDisplay error={getApiError(save.error)} />
						</Box>
					) : null}
				</Stack>
			) : null}
		</Modal>
	);
}
