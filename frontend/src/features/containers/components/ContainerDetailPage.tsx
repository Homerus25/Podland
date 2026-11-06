import { EditIcon } from "@chakra-ui/icons";
import {
	Button,
	Link as ChakraLink,
	HStack,
	Table,
	TableContainer,
	Tbody,
	Td,
	Text,
	Tr,
	VStack,
} from "@chakra-ui/react";
import { ContainerStatsDialog } from "@features/containers/components/ContainerStatsDialog";
import {
	useContainer,
	useContainerInspect,
	useDeleteContainer,
	useRenameContainer,
	useUpdateContainer,
} from "@features/containers/hooks/useContainers";
import type { ContainerDetail } from "@features/containers/services/containersApi";
import {
	type CapabilityGate,
	useCapabilityGate,
} from "@features/hosts/hooks/useCapabilityGate";
import { DetailPage } from "@shared/components/DetailPage";
import {
	DetailList,
	DetailRow,
	isNotFound,
	removalAction,
} from "@shared/components/detailKit";
import { ForceConfirmDialog } from "@shared/components/MutationDialog";
import { PermissionHint } from "@shared/components/PermissionHint";
import { StateBadge } from "@shared/components/StateBadge";
import { detailPath, hostTabPath } from "@shared/routes/detailRoute";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import {
	Link as ReactRouterLink,
	useNavigate,
	useParams,
} from "react-router-dom";
import { CONTAINER_RUNNING_REFUSAL } from "./containerRemoval";
import { EditConfigurationDialog } from "./EditConfigurationDialog";
import { RenameContainerDialog } from "./RenameContainerDialog";

// The detail page's Remove adapter over the shared force-refusal preset
// (spec 461, ticket #467). The preset owns the ritual — the refusal-code
// predicate, the hint replacing the error region, the confirm-to-force
// footer swap, the pending wiring, and the force flag keyed off the current
// error — so this adapter keeps only the domain: the refusal vocabulary
// (containerRemoval.ts), the labels, and the payload builder taking the
// force flag. Success navigates away (the overview query would 404 next).
function RemoveContainerDialog({
	container,
	removeContainer,
	onClose,
	onRemoved,
}: {
	container: ContainerDetail;
	removeContainer: ReturnType<typeof useDeleteContainer>;
	onClose: () => void;
	onRemoved: () => void;
}) {
	return (
		<ForceConfirmDialog
			mutation={removeContainer}
			title="Remove container"
			onClose={onClose}
			refusalCode={CONTAINER_RUNNING_REFUSAL.code}
			hint={CONTAINER_RUNNING_REFUSAL.hint}
			hintTestId={CONTAINER_RUNNING_REFUSAL.hintTestId}
			confirmLabel="Remove container"
			forceLabel="Force remove"
			forceTestId="force-remove-button"
			confirm={(force) => ({ containerId: container.id, force })}
			onSuccess={onRemoved}
			body={
				<Text>
					Remove “{container.name}” ({container.image})? This cannot be undone.
				</Text>
			}
		/>
	);
}

// The container's overview: one label/value row per attribute the spec
// lists. Cross-links (image, volumes, secrets) point at the related
// resources' detail pages. The Name row carries the rename control: a pencil button
// for callers who can manage containers, the shared write-permission hint
// for everyone else — the secret detail page's hidden-admin-control
// pattern, so a read-only caller is never baited into a 403. The gate is
// the capability-gate hook's tri-state (the reference adapter of the
// detail-page gating settlement): while the caller's level is pending the
// row renders neither control, rather than flashing the read-only hint
// at a write-capable caller.
function ContainerOverview({
	hostId,
	container,
	renameAccess,
	onRename,
}: {
	hostId: string;
	container: ContainerDetail;
	renameAccess: CapabilityGate;
	onRename: () => void;
}) {
	const environment = Object.entries(container.environment ?? {});
	// The healthcheck's timing line: only the fields the container
	// carries, joined into one gray line under the command.
	const healthcheckTiming = container.healthcheck
		? [
				container.healthcheck.interval
					? `Interval ${container.healthcheck.interval}`
					: null,
				container.healthcheck.timeout
					? `Timeout ${container.healthcheck.timeout}`
					: null,
				container.healthcheck.startPeriod
					? `Start period ${container.healthcheck.startPeriod}`
					: null,
				container.healthcheck.retries != null
					? `Retries ${container.healthcheck.retries}`
					: null,
			]
				.filter((part) => part !== null)
				.join(" · ")
		: "";
	return (
		<TableContainer data-testid="container-detail-overview">
			<Table size="sm">
				<Tbody>
					<DetailRow label="Name">
						<HStack spacing={2} align="center">
							<Text fontFamily="mono" data-testid="container-detail-name">
								{container.name}
							</Text>
							{renameAccess === "allowed" ? (
								<Button
									size="xs"
									variant="ghost"
									aria-label={`Rename ${container.name}`}
									onClick={onRename}
									data-testid="rename-container-button"
								>
									<EditIcon />
								</Button>
							) : null}
						</HStack>
						{renameAccess === "denied" ? (
							<PermissionHint
								scope="rename"
								requirement="write"
								resource="container"
								fontSize="sm"
							/>
						) : null}
					</DetailRow>
					<DetailRow label="Id">
						<Text fontFamily="mono" data-testid="container-detail-id">
							{container.id}
						</Text>
					</DetailRow>
					<DetailRow label="State">
						<StateBadge state={container.state} />
					</DetailRow>
					<DetailRow label="ImageName">
						{container.imageId ? (
							<ChakraLink
								as={ReactRouterLink}
								to={detailPath(hostId, "images", container.imageId)}
								color="blue.500"
								data-testid="container-image-link"
							>
								{container.image}
							</ChakraLink>
						) : (
							<Text>{container.image}</Text>
						)}
					</DetailRow>
					<DetailRow label="Volumes">
						<DetailList
							items={container.volumes}
							getKey={(volume) => volume.name}
							testId="container-detail-volumes"
							renderItem={(volume) => (
								<>
									<ChakraLink
										as={ReactRouterLink}
										to={detailPath(hostId, "volumes", volume.name)}
										color="blue.500"
										data-testid={`container-volume-link-${volume.name}`}
									>
										{volume.name}
									</ChakraLink>
									: {volume.source} → {volume.destination} (
									{volume.rw ? "rw" : "ro"})
								</>
							)}
						/>
					</DetailRow>
					<DetailRow label="Mounts">
						<DetailList
							items={container.mounts}
							getKey={(mount) => mount}
							testId="container-detail-mounts"
							mono
							renderItem={(mount) => mount}
						/>
					</DetailRow>
					<DetailRow label="Networks">
						<DetailList
							items={container.networks}
							getKey={(network) => network}
							testId="container-detail-networks"
							renderItem={(network) => network}
						/>
					</DetailRow>
					<DetailRow label="Exposed Ports">
						<DetailList
							items={container.exposedPorts}
							getKey={(port) => `${port.containerPort}/${port.protocol}`}
							testId="container-detail-exposed-ports"
							renderItem={(port) => `${port.containerPort}/${port.protocol}`}
						/>
					</DetailRow>
					<DetailRow label="Entrypoint">
						<Text data-testid="container-detail-entrypoint">
							{container.entrypoint && container.entrypoint.length > 0
								? container.entrypoint.join(" ")
								: "None"}
						</Text>
					</DetailRow>
					<DetailRow label="Environment Variables">
						{environment.length === 0 ? (
							<Text color="gray.500">None</Text>
						) : (
							<TableContainer data-testid="container-detail-environment">
								<Table size="sm">
									<Tbody>
										{environment.map(([key, value]) => (
											<Tr key={key}>
												<Td fontFamily="mono" width="40%">
													{key}
												</Td>
												<Td fontFamily="mono">{value}</Td>
											</Tr>
										))}
									</Tbody>
								</Table>
							</TableContainer>
						)}
					</DetailRow>
					<DetailRow label="Secrets">
						<DetailList
							items={container.secrets}
							getKey={(secret) => secret.name}
							testId="container-detail-secrets"
							renderItem={(secret) => (
								<ChakraLink
									as={ReactRouterLink}
									to={detailPath(hostId, "secrets", secret.name)}
									color="blue.500"
									data-testid={`container-secret-link-${secret.name}`}
								>
									{secret.name}
								</ChakraLink>
							)}
						/>
					</DetailRow>
					<DetailRow label="Created">
						<Text data-testid="container-detail-created">
							{formatRelativeDate(container.createdAt)}
						</Text>
					</DetailRow>
					<DetailRow label="Cmd">
						<Text data-testid="container-detail-command">
							{container.command && container.command.length > 0
								? container.command.join(" ")
								: "None"}
						</Text>
					</DetailRow>
					{/* The current limits and healthcheck (spec 412): what the
					edit-configuration dialog prefills from and what a
					successful update refreshes. */}
					<DetailRow label="CPUs">
						<Text data-testid="container-detail-cpus">
							{container.limits?.cpus ? container.limits.cpus : "None"}
						</Text>
					</DetailRow>
					<DetailRow label="Memory">
						<Text fontFamily="mono" data-testid="container-detail-memory">
							{container.limits?.memory ? container.limits.memory : "None"}
						</Text>
					</DetailRow>
					<DetailRow label="Healthcheck">
						{container.healthcheck ? (
							<VStack
								spacing={0}
								align="start"
								data-testid="container-detail-healthcheck"
							>
								<Text fontFamily="mono">
									{container.healthcheck.disabled
										? "Disabled"
										: container.healthcheck.command || "None"}
								</Text>
								{!container.healthcheck.disabled && healthcheckTiming ? (
									<Text fontSize="sm" color="gray.500">
										{healthcheckTiming}
									</Text>
								) : null}
							</VStack>
						) : (
							<Text color="gray.500">None</Text>
						)}
					</DetailRow>
				</Tbody>
			</Table>
		</TableContainer>
	);
}

// The container detail page (spec 301, ticket #306): the shared layout's
// shell with this feature's overview rows. The overview query rides the
// shared freshness factory's one-shot flavor, so every visit refetches;
// the raw inspect report fetches lazily on the
// first "Show inspect JSON" click, never on page mount. The capability
// gate (the hosts feature's hook) answers the per-caller facts both
// gated affordances render from; the overview itself is readable at read
// level.
export function ContainerDetailPage() {
	const { hostId, containerId } = useParams<{
		hostId: string;
		containerId: string;
	}>();
	const query = useContainer(hostId!, containerId!);
	const [revealed, setRevealed] = useState(false);
	const [renaming, setRenaming] = useState(false);
	const [statsOpen, setStatsOpen] = useState(false);
	const [editingConfig, setEditingConfig] = useState(false);
	const [removing, setRemoving] = useState(false);
	const inspectQuery = useContainerInspect(hostId!, containerId!, {
		enabled: revealed,
	});
	const gate = useCapabilityGate(hostId!, "manage-containers");
	const rename = useRenameContainer(hostId!, containerId!);
	const update = useUpdateContainer(hostId!, containerId!);
	const removeContainer = useDeleteContainer(hostId!);
	const navigate = useNavigate();
	const container = query.data;
	const notFound = isNotFound(query.error);

	// The header's Remove action (spec 461, ticket #467) rides the detail
	// kit's gate cascade: the button on allowed, the shared write-permission
	// hint on denied, neither while pending. The header actions sit outside
	// the layout's state ternary, so the action also hides itself once the
	// overview 404s — there is nothing left to remove.
	const removeAction = removalAction({
		gate,
		notFound,
		scope: "containers",
		resource: "container",
		testId: "remove-container-button",
		onClick: () => setRemoving(true),
	});

	return (
		<>
			<DetailPage
				title={container?.name ?? containerId!}
				backTo="containers"
				isPending={query.isPending}
				error={query.error}
				notFound={notFound}
				inspectQuery={inspectQuery}
				revealed={revealed}
				onToggleInspect={() => setRevealed((value) => !value)}
				headerActions={
					// The edit-configuration action (spec 412, ticket #414),
					// riding the same capability gate as the rename control:
					// rendered on allowed, nothing otherwise — hidden below
					// write level and while the gate is pending, so a
					// read-only caller is never baited into a 403.
					gate === "allowed" || removeAction ? (
						<>
							{gate === "allowed" ? (
								<Button
									size="sm"
									onClick={() => setEditingConfig(true)}
									data-testid="edit-configuration-button"
								>
									Edit configuration
								</Button>
							) : null}
							{removeAction}
						</>
					) : undefined
				}
			>
				{container ? (
					<>
						{/* Stats is a read, so the button renders for every caller.
						It sits feature-locally at the top of the content area —
						the shared layout's header actions are deliberately not
						extended (ticket #414 owns that slot). */}
						<HStack justify="flex-end" mb={2}>
							<Button
								size="sm"
								variant="outline"
								onClick={() => setStatsOpen(true)}
								data-testid="container-stats-button"
							>
								Stats
							</Button>
						</HStack>
						<ContainerOverview
							hostId={hostId!}
							container={container}
							renameAccess={gate}
							onRename={() => setRenaming(true)}
						/>
					</>
				) : null}
			</DetailPage>
			{statsOpen ? (
				<ContainerStatsDialog
					hostId={hostId!}
					containerId={containerId!}
					onClose={() => setStatsOpen(false)}
				/>
			) : null}
			{renaming && container ? (
				<RenameContainerDialog
					currentName={container.name}
					mutation={rename}
					onClose={() => setRenaming(false)}
				/>
			) : null}
			{editingConfig && container ? (
				<EditConfigurationDialog
					container={container}
					mutation={update}
					onClose={() => setEditingConfig(false)}
				/>
			) : null}
			{removing && container ? (
				<RemoveContainerDialog
					container={container}
					removeContainer={removeContainer}
					onClose={() => setRemoving(false)}
					onRemoved={() => navigate(hostTabPath(hostId!, "containers"))}
				/>
			) : null}
		</>
	);
}
