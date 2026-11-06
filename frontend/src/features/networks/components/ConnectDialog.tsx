import {
	Alert,
	AlertDescription,
	AlertIcon,
	AlertTitle,
	Box,
	FormControl,
	FormErrorMessage,
	FormLabel,
	List,
	ListItem,
} from "@chakra-ui/react";
import {
	type ContainerNamed,
	containerDisplayName,
} from "@features/containers/components/containerDisplayName";
import { useContainers } from "@features/containers/hooks/useContainers";
import { runningContainers } from "@features/networks/components/containerOptions";
import {
	ConnectError,
	type ConnectFailure,
	useConnectContainers,
	useNetworks,
} from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import {
	FormDialog,
	FormValidityScope,
} from "@shared/components/MutationDialog";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import { useForm } from "react-hook-form";

interface ConnectFormValues {
	pick: string[];
}

// The container-fixed side reads exactly three facts off the container it is
// pinned to: its id, its display name (the shared naming rule reads `names`
// or `name`), and the networks it is already attached to. The prop is
// ContainerNamed — the naming rule's own shape — plus the networks, so the
// containers list's summary and the container detail page's detail DTO both
// satisfy it as they are — no projection, and nothing silently dropped on
// the way in.
export type ConnectContainer = ContainerNamed & {
	networks?: string[];
};

// The fixed side discriminates the flow: the Networks tab passes the
// network and the dialog picks the containers; the Containers tab passes
// the container and the dialog picks the networks.
type ConnectSide =
	| { kind: "network"; network: NetworkSummary }
	| { kind: "container"; container: ConnectContainer };

type ConnectDialogProps = {
	hostId: string;
	onClose: () => void;
} & (
	| { network: NetworkSummary; container?: never }
	| { container: ConnectContainer; network?: never }
);

// The one owner of the connect ritual (issue 274), multi-select since
// issue 320: both tabs' connect flows ride this dialog, entered with the
// fixed side and a multi-picker for the other. The dialog fetches its own
// options and drives the connect mutation itself — one request per picked
// item, both lists invalidated once after all requests settle. Options
// that can only produce an error never appear: the container side offers
// running containers only, already-attached ones excluded; the network
// side offers the networks the container is not on. The picked side is a
// required form field ("at least one"), so the module footer's submit
// stays disabled until something is picked; confirm sends one
// { networkName, containerId } pair per pick. A partial failure renders
// the aggregated summary in the module's error region, prunes the
// selection to the failed items for retry, and keeps the dialog open;
// only an all-success batch closes it.
export function ConnectDialog(props: ConnectDialogProps) {
	const { hostId, onClose } = props;
	const side: ConnectSide =
		props.network !== undefined
			? { kind: "network", network: props.network }
			: { kind: "container", container: props.container };

	const copy =
		side.kind === "network"
			? {
					title: `Connect container to ${side.network.name}`,
					formId: "connect-container-form",
					submitTestId: "connect-container-submit",
					label: "Containers",
					requiredMessage: "Select at least one container",
					searchPlaceholder: "Search containers…",
					emptyMessage: "No running containers to connect.",
				}
			: {
					title: `Connect ${containerDisplayName(side.container)} to a network`,
					formId: "connect-network-form",
					submitTestId: "connect-network-submit",
					label: "Networks",
					requiredMessage: "Select at least one network",
					searchPlaceholder: "Search networks…",
					emptyMessage: "No networks to connect.",
				};

	const { data: containers } = useContainers(hostId);
	const { data: networks } = useNetworks(hostId);
	const connect = useConnectContainers(hostId);

	const { register, handleSubmit, setValue, watch, formState } =
		useForm<ConnectFormValues>({
			defaultValues: { pick: [] },
			// onChange so the footer submit's disabled state tracks validity
			// as the user picks, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;
	const selected = watch("pick");
	// The select is not a native input, so the field registers without
	// props; setValue below drives it and shouldValidate keeps the footer's
	// submit policy in step with the picks.
	register("pick", {
		validate: (value) => value.length > 0 || copy.requiredMessage,
	});

	// Already-attached = the container's networks include the fixed
	// network; the container's current networks are excluded on the other
	// side — an entry that can only produce an error is bait.
	const options =
		side.kind === "network"
			? runningContainers(containers)
					.filter(
						(container) =>
							!(container.networks ?? []).includes(side.network.name),
					)
					.map((container) => ({
						value: container.id,
						label: containerDisplayName(container),
					}))
			: (networks ?? [])
					.filter(
						(network) =>
							!(side.container.networks ?? []).includes(network.name),
					)
					.map((network) => ({ value: network.name, label: network.name }));

	// The picked value of a failed pair, in the option list's vocabulary:
	// the container id on the network-fixed side, the network name on the
	// container-fixed side.
	const pickedValue = (failure: ConnectFailure) =>
		side.kind === "network" ? failure.containerId : failure.networkName;

	const submit = handleSubmit((values) => {
		connect.mutate(
			side.kind === "network"
				? values.pick.map((containerId) => ({
						networkName: side.network.name,
						containerId,
					}))
				: values.pick.map((networkName) => ({
						networkName,
						containerId: side.container.id,
					})),
		);
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={connect}
				title={copy.title}
				onClose={onClose}
				formId={copy.formId}
				submitLabel="Connect"
				submitTestId={copy.submitTestId}
				// A partial failure prunes the selection to the failed items,
				// so a retry fires only those and the succeeded connections
				// are not re-requested.
				onError={(error) => {
					if (!(error instanceof ConnectError)) return;
					setValue("pick", error.failures.map(pickedValue), {
						shouldValidate: true,
					});
				}}
				// The seam's parsed envelope collapses the aggregate —
				// getApiError passes through axios errors only — so the summary
				// reads the failures off the same mutation result the dialog
				// hands the module as `mutation` above.
				renderError={() => (
					<ConnectErrorSummary
						failures={connect.error?.failures ?? []}
						labelFor={(failure) =>
							options.find((option) => option.value === pickedValue(failure))
								?.label ?? pickedValue(failure)
						}
					/>
				)}
				body={
					<form id={copy.formId} onSubmit={submit} noValidate>
						<FormControl isInvalid={!!errors.pick} isRequired>
							<FormLabel>{copy.label}</FormLabel>
							<SearchableSelect
								mode="multi"
								options={options}
								value={selected}
								onChange={(value) =>
									setValue("pick", value, { shouldValidate: true })
								}
								searchPlaceholder={copy.searchPlaceholder}
								emptyMessage={copy.emptyMessage}
							/>
							<FormErrorMessage>{errors.pick?.message}</FormErrorMessage>
						</FormControl>
					</form>
				}
			/>
		</FormValidityScope>
	);
}

// The partial-failure summary (issue 320): the module's error region is
// replaced by a per-item list — every failed pair named by its option
// label with the backend's message — so the user sees exactly what to
// retry.
function ConnectErrorSummary({
	failures,
	labelFor,
}: {
	failures: ConnectFailure[];
	labelFor: (failure: ConnectFailure) => string;
}) {
	return (
		<Alert
			status="error"
			variant="subtle"
			role="alert"
			data-testid="mutation-dialog-error"
		>
			<AlertIcon />
			<Box>
				<AlertTitle>
					{failures.length === 1
						? "1 connection failed"
						: `${failures.length} connections failed`}
				</AlertTitle>
				<AlertDescription display="block">
					<List spacing={1}>
						{failures.map((failure) => (
							<ListItem key={`${failure.networkName}/${failure.containerId}`}>
								{labelFor(failure)}: {failure.error.message}
							</ListItem>
						))}
					</List>
				</AlertDescription>
			</Box>
		</Alert>
	);
}
