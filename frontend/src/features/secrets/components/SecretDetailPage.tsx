import { Box, Button, HStack, Table, Tbody, Text } from "@chakra-ui/react";
import {
	type CapabilityGate,
	useCapabilityGate,
} from "@features/hosts/hooks/useCapabilityGate";
import {
	useDeleteSecret,
	useRevealSecret,
	useSecret,
	useSecretInspect,
} from "@features/secrets/hooks/useSecrets";
import type { SecretInspect } from "@features/secrets/services/secretsApi";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailRow,
	isNotFound,
	removalAction,
} from "@shared/components/detailKit";
import { ErrorDisplay } from "@shared/components/ErrorDisplay";
import { ConfirmDialog } from "@shared/components/MutationDialog";
import { PermissionHint } from "@shared/components/PermissionHint";
import { hostTabPath } from "@shared/routes/detailRoute";
import { getApiError } from "@shared/utils/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

const MASK = "••••••••••••";

// The backend's refusal sentinel for removing a secret a container still
// references. Secrets have no force parameter at all, so the refusal is
// explained and the plain confirm stays — there is no forced retry to offer.
const SECRET_IN_USE_CODE = "SECRET_IN_USE";

// The detail page's Remove adapter over the plain confirm preset (spec 461,
// ticket #467). The module owns the shell, the reset lifecycle, the danger
// footer, the pending wiring, close-on-success, and the error region; this
// adapter keeps the domain body, the variables (the secret's name — the
// endpoint addresses secrets by name), and the in-use hint. The hint rides
// the renderError seam, which replaces the region rather than appending to
// it, so the adapter composes the shared ErrorDisplay plus the hint itself
// (the retired volume modal's composition, minus the force path).
function RemoveSecretDialog({
	secret,
	removeSecret,
	onClose,
	onRemoved,
}: {
	secret: SecretInspect;
	removeSecret: ReturnType<typeof useDeleteSecret>;
	onClose: () => void;
	onRemoved: () => void;
}) {
	return (
		<ConfirmDialog
			mutation={removeSecret}
			title="Remove secret"
			confirmLabel="Remove secret"
			confirm={() => secret.name}
			onClose={onClose}
			onSuccess={onRemoved}
			body={<Text>Remove “{secret.name}”? This cannot be undone.</Text>}
			renderError={(apiError) => (
				<>
					<ErrorDisplay error={apiError} />
					{apiError.code === SECRET_IN_USE_CODE ? (
						<Text mt={2} color="orange.500" data-testid="secret-in-use-warning">
							One or more containers use this secret, so it cannot be removed.
							Remove those containers first.
						</Text>
					) : null}
				</>
			)}
		/>
	);
}

// The reveal-value flow, preserved from the former SecretDetailModal (spec
// 301): the value travels through a mutation, so React Query never caches
// it and it dies with the page. On screen the value stays masked until a
// second click on the eye. The section renders from the capability gate's
// tri-state: the flow on allowed, the admin hint on denied — including a
// failed host query — and neither while pending, so an admin caller
// never sees the hint flash while their level loads.
function RevealValueSection({
	hostId,
	secretId,
	revealAccess,
}: {
	hostId: string;
	secretId: string;
	revealAccess: CapabilityGate;
}) {
	const reveal = useRevealSecret(hostId);
	const [isVisible, setIsVisible] = useState(false);

	if (revealAccess === "pending") {
		return null;
	}

	return (
		<Box borderTopWidth={1} pt={3}>
			{reveal.error ? (
				<Box mb={2}>
					<ErrorDisplay error={getApiError(reveal.error)} />
				</Box>
			) : null}
			{revealAccess === "allowed" ? (
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
// detail query (the freshness factory's one-shot flavor, refetched on
// every visit); the raw inspect
// report fetches lazily on the first "Show inspect JSON" click and never
// carries the value. The capability gate (the hosts feature's hook)
// answers the per-caller fact the reveal flow renders from; the overview
// itself is readable at read level. The header's Remove action (spec 461,
// ticket #467) rides a second gate on the manage capability, since removing
// is a write while revealing is admin.
export function SecretDetailPage() {
	const { hostId, secretId } = useParams<{
		hostId: string;
		secretId: string;
	}>();
	const [revealed, setRevealed] = useState(false);
	const [removing, setRemoving] = useState(false);
	const secret = useSecret(hostId!, secretId!);
	const inspectQuery = useSecretInspect(hostId!, secretId!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "reveal-secrets");
	const removeGate = useCapabilityGate(hostId!, "manage-secrets");
	const removeSecret = useDeleteSecret(hostId!);
	const navigate = useNavigate();
	const notFound = isNotFound(secret.error);

	// The header's Remove action rides the detail kit's gate cascade: the
	// button on allowed, the shared write-permission hint on denied, neither
	// while pending. The header actions sit outside the layout's state
	// ternary, so the action also hides itself once the overview 404s —
	// there is nothing left to remove.
	const removeAction = removalAction({
		gate: removeGate,
		notFound,
		scope: "secrets",
		resource: "secret",
		testId: "remove-secret-button",
		onClick: () => setRemoving(true),
	});

	return (
		<>
			<DetailPage
				title={secret.data?.name ?? secretId!}
				backTo="secrets"
				isPending={secret.isPending}
				error={secret.error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={removeAction}
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
											{Object.entries(secret.data.labels).map(
												([key, value]) => (
													<Text key={key} fontFamily="mono">
														{key}={value}
													</Text>
												),
											)}
										</HStack>
									</DetailRow>
								) : null}
							</Tbody>
						</Table>
						<RevealValueSection
							hostId={hostId!}
							secretId={secretId!}
							revealAccess={gate}
						/>
					</>
				) : null}
			</DetailPage>
			{removing && secret.data ? (
				<RemoveSecretDialog
					secret={secret.data}
					removeSecret={removeSecret}
					onClose={() => setRemoving(false)}
					onRemoved={() => navigate(hostTabPath(hostId!, "secrets"))}
				/>
			) : null}
		</>
	);
}
