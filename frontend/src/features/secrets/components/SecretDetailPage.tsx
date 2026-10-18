import { Box, Button, HStack, Table, Tbody, Text } from "@chakra-ui/react";
import { useMachine } from "@features/machines/hooks/useMachines";
import { can } from "@features/machines/permissions";
import {
	useRevealSecret,
	useSecret,
	useSecretInspect,
} from "@features/secrets/hooks/useSecrets";
import { DetailPage } from "@shared/components/DetailPage";
import { DetailRow, isNotFound } from "@shared/components/detailKit";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { PermissionHint } from "@shared/components/PermissionHint";
import type { Machine } from "@shared/types/api";
import { getApiError } from "@shared/utils/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useParams } from "react-router-dom";

const MASK = "••••••••••••";

// The reveal-value flow, preserved from the former SecretDetailModal (spec
// 301): the value travels through a mutation, so React Query never caches
// it and it dies with the page. On screen the value stays masked until a
// second click on the eye. Admin-gated: below admin the hint replaces the
// control, so users are never baited into a 403.
function RevealValueSection({
	machine,
	secretId,
}: {
	machine: Machine;
	secretId: string;
}) {
	const reveal = useRevealSecret(machine.id);
	const [isVisible, setIsVisible] = useState(false);
	const canReveal = can(machine, "reveal-secrets");

	return (
		<Box borderTopWidth={1} pt={3}>
			{reveal.error ? (
				<Box mb={2}>
					<ErrorDisplay error={getApiError(reveal.error)} />
				</Box>
			) : null}
			{canReveal ? (
				reveal.data ? (
					<HStack>
						<Text
							fontFamily="mono"
							data-testid="secret-value"
							data-visible={isVisible}
						>
							{isVisible ? reveal.data.secretData : MASK}
						</Text>
						<Button
							size="xs"
							variant="ghost"
							aria-label={isVisible ? "Hide value" : "Show value"}
							onClick={() => setIsVisible((v) => !v)}
							data-testid="toggle-secret-visibility"
						>
							{isVisible ? "🙈" : "👁"}
						</Button>
					</HStack>
				) : (
					<Button
						size="sm"
						onClick={() => {
							reveal.reset();
							setIsVisible(false);
							reveal.mutate(secretId);
						}}
						data-testid="reveal-secret-button"
					>
						Reveal value
					</Button>
				)
			) : (
				<PermissionHint scope="reveal" requirement="admin" fontSize="sm" />
			)}
		</Box>
	);
}

// The secret detail page (spec 301, ticket #310): the shared layout's shell
// with the secret's metadata as overview rows and the former modal's
// reveal-value flow preserved below them. The overview is the useSecret
// detail query (gcTime: 0, refetched on every visit); the raw inspect
// report fetches lazily on the first "Show inspect JSON" click and never
// carries the value. The machine query supplies the per-caller facts the
// reveal gate reads; the overview itself is readable at read level.
export function SecretDetailPage() {
	const { machineId, secretId } = useParams<{
		machineId: string;
		secretId: string;
	}>();
	const [revealed, setRevealed] = useState(false);
	const secret = useSecret(machineId!, secretId!);
	const inspectQuery = useSecretInspect(machineId!, secretId!, {
		enabled: revealed,
	});
	const { data: machine, error: machineError } = useMachine(machineId!);

	return (
		<DetailPage
			title={secret.data?.name ?? secretId!}
			backTo="secrets"
			isPending={secret.isPending}
			error={secret.error}
			notFound={isNotFound(secret.error)}
			inspectQuery={inspectQuery}
			revealed={revealed}
			onToggleInspect={() => setRevealed((value) => !value)}
		>
			{secret.data ? (
				<>
					<Table size="sm">
						<Tbody>
							<DetailRow label="ID">
								<Text fontFamily="mono" data-testid="secret-detail-id">
									{secret.data.id}
								</Text>
							</DetailRow>
							<DetailRow label="Name">
								<Text fontFamily="mono" data-testid="secret-detail-name">
									{secret.data.name}
								</Text>
							</DetailRow>
							<DetailRow label="Driver">
								<Text data-testid="secret-detail-driver">
									{secret.data.driver}
								</Text>
							</DetailRow>
							<DetailRow label="Created">
								<Text data-testid="secret-detail-created">
									{formatRelativeDate(secret.data.createdAt)}
								</Text>
							</DetailRow>
							<DetailRow label="Updated">
								<Text data-testid="secret-detail-updated">
									{formatRelativeDate(secret.data.updatedAt)}
								</Text>
							</DetailRow>
							{secret.data.labels &&
							Object.keys(secret.data.labels).length > 0 ? (
								<DetailRow label="Labels">
									<HStack data-testid="secret-detail-labels">
										{Object.entries(secret.data.labels).map(([key, value]) => (
											<Text key={key} fontFamily="mono">
												{key}={value}
											</Text>
										))}
									</HStack>
								</DetailRow>
							) : null}
						</Tbody>
					</Table>
					{machine ? (
						<RevealValueSection machine={machine} secretId={secretId!} />
					) : machineError ? (
						// The machine query failed, so the caller's permission level
						// is unknown — render the hint rather than nothing, so a
						// non-admin never sees a bare gap (ticket #310).
						<Box borderTopWidth={1} pt={3}>
							<PermissionHint
								scope="reveal"
								requirement="admin"
								fontSize="sm"
							/>
						</Box>
					) : null}
				</>
			) : null}
		</DetailPage>
	);
}
