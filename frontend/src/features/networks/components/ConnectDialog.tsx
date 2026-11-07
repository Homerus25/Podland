import {
	Alert,
	AlertDescription,
	AlertIcon,
	AlertTitle,
	Box,
	Button,
	FormControl,
	FormErrorMessage,
	FormLabel,
	List,
	ListItem,
	Table,
	TableContainer,
	Tbody,
	Td,
	Th,
	Thead,
	Tr,
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
	useSequentialConnect,
} from "@features/networks/hooks/useNetworks";
import type { NetworkSummary } from "@features/networks/services/networksApi";
import { Modal } from "@shared/components/Modal";
import {
	FormDialog,
	FormValidityScope,
} from "@shared/components/MutationDialog";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import type { ApiError } from "@shared/types/api";
import { type FormEvent, useRef, useState } from "react";
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
// the container and the dialog picks the networks; the containers list's
// bulk action passes the whole selection and the dialog picks one network
// for all of them (spec #486, ticket #491).
type ConnectFixedSide =
	| { kind: "network"; network: NetworkSummary }
	| { kind: "container"; container: ConnectContainer };

type ConnectDialogProps = {
	hostId: string;
	onClose: () => void;
} & (
	| { network: NetworkSummary; container?: never; containers?: never }
	| { container: ConnectContainer; network?: never; containers?: never }
	| {
			containers: ConnectContainer[];
			network?: never;
			container?: never;
			// The bulk run's completion seam: fired exactly once after the
			// last pair settles, carrying the ids that succeeded — the
			// caller clears exactly those from its selection and leaves the
			// failures selected for retry.
			onFinished: (succeededKeys: string[]) => void;
	  }
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
//
// The third fixed side (spec #486, ticket #491) is the containers list's bulk
// action: the whole selection is fixed and one network is chosen for all of
// it, then the connects run sequentially with a per-item report. It is a
// different shape from the two above — see ConnectSelectedDialog — and the
// two single-fixed-side flows are untouched by it.
export function ConnectDialog(props: ConnectDialogProps) {
	// The bulk variant is a different shape — one network chosen once, then a
	// per-item report as the connects run — so it is its own component over
	// the shared Modal shell rather than a third branch of the form dialog.
	if (props.containers !== undefined) {
		return (
			<ConnectSelectedDialog
				hostId={props.hostId}
				containers={props.containers}
				onClose={props.onClose}
				onFinished={props.onFinished}
			/>
		);
	}
	const side: ConnectFixedSide =
		props.network !== undefined
			? { kind: "network", network: props.network }
			: { kind: "container", container: props.container };
	return (
		<ConnectFixedSideDialog
			hostId={props.hostId}
			onClose={props.onClose}
			side={side}
		/>
	);
}

// The two single-fixed-side flows (issue 274, multi-select since issue 320):
// the Networks tab fixes the network and picks containers, the Containers tab
// fixes the container and picks networks. Unchanged by the bulk variant.
function ConnectFixedSideDialog({
	hostId,
	onClose,
	side,
}: {
	hostId: string;
	onClose: () => void;
	side: ConnectFixedSide;
}) {
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

// One row of the bulk run's report: the container, and its outcome once that
// container's request settles (`settled` false while it is still in flight).
type ConnectReportRow = {
	containerId: string;
	settled: boolean;
	failure?: ApiError;
};

// The bulk variant (spec #486, ticket #491): the containers list's "Connect
// selected to network". One network is chosen once for the whole selection —
// no per-container confirmation — and the connects then run one at a time in
// list order over the same per-item endpoint, each row's result filling in as
// that container settles. It is a two-phase flow (form → report) that no
// MutationDialog preset fits, so it is a feature-owned modal over the shared
// Modal shell, like ImportComposeModal: the list renders it conditionally, so
// mount is open and unmount is close, and reopening always starts clean.
//
// The options follow the single-container rule applied across the selection:
// a network EVERY selected container is already attached to is excluded,
// because connecting to it could only produce an error. A network only some
// of them are on stays — those connects can succeed.
//
// `onFinished` fires exactly once, after the last pair settles, carrying the
// ids that succeeded: the caller clears exactly those from its selection and
// leaves the failures selected for retry. An abandoned run reports nothing —
// closing mid-run aborts the in-flight request and the remaining pairs never
// start, the same rule the batch transfer dialog pins (spec 486 review).
function ConnectSelectedDialog({
	hostId,
	containers,
	onClose,
	onFinished,
}: {
	hostId: string;
	containers: ConnectContainer[];
	onClose: () => void;
	onFinished: (succeededKeys: string[]) => void;
}) {
	const { data: networks } = useNetworks(hostId);
	const { run } = useSequentialConnect(hostId);
	const [networkName, setNetworkName] = useState<string | null>(null);
	// The report is the run's own narration: one row per selected container in
	// list order, its result filled in as that container settles. Null until
	// the run starts — the body is the picker until then.
	const [report, setReport] = useState<ConnectReportRow[] | null>(null);
	const abortRef = useRef<AbortController | null>(null);

	// Closing mid-run aborts the in-flight connect and closes: the run's
	// remaining containers never start, and the list hears nothing back.
	const close = () => {
		abortRef.current?.abort();
		abortRef.current = null;
		onClose();
	};

	const options = (networks ?? [])
		.filter(
			(network) =>
				!containers.every((container) =>
					(container.networks ?? []).includes(network.name),
				),
		)
		.map((network) => ({ value: network.name, label: network.name }));

	const labelFor = (containerId: string) => {
		const container = containers.find((entry) => entry.id === containerId);
		return container ? containerDisplayName(container) : containerId;
	};

	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (networkName === null) return;
		const controller = new AbortController();
		abortRef.current = controller;
		const succeeded: string[] = [];
		setReport(
			containers.map((container) => ({
				containerId: container.id,
				settled: false,
			})),
		);
		void run(
			containers.map((container) => ({
				networkName,
				containerId: container.id,
			})),
			(containerId, failure) => {
				if (!failure) succeeded.push(containerId);
				setReport(
					(current) =>
						current?.map((row) =>
							row.containerId === containerId
								? { ...row, settled: true, failure }
								: row,
						) ?? current,
				);
			},
			controller.signal,
		).then(() => {
			abortRef.current = null;
			// An abandoned run reports nothing: the dialog closed, so the
			// list keeps its selection untouched.
			if (!controller.signal.aborted) onFinished(succeeded);
		});
	};

	return (
		<Modal
			isOpen
			onClose={close}
			title={`Connect ${containers.length} containers to a network`}
			footer={
				report ? (
					<Button
						onClick={close}
						data-testid="close-connect-selected-report-button"
					>
						Close
					</Button>
				) : (
					<>
						<Button variant="ghost" onClick={close}>
							Cancel
						</Button>
						<Button
							type="submit"
							form="connect-selected-form"
							colorScheme="blue"
							isDisabled={networkName === null}
							data-testid="connect-selected-submit"
						>
							Connect
						</Button>
					</>
				)
			}
		>
			{report ? (
				<TableContainer>
					<Table data-testid="connect-selected-report" size="sm">
						<Thead>
							<Tr>
								<Th>Item</Th>
								<Th>Result</Th>
							</Tr>
						</Thead>
						<Tbody>
							{report.map((row) => (
								<Tr
									key={row.containerId}
									data-testid="connect-selected-report-row"
								>
									<Td fontFamily="mono">{labelFor(row.containerId)}</Td>
									<Td>
										{row.settled
											? row.failure
												? row.failure.message
												: "Connected"
											: "Connecting…"}
									</Td>
								</Tr>
							))}
						</Tbody>
					</Table>
				</TableContainer>
			) : (
				<form id="connect-selected-form" onSubmit={submit} noValidate>
					<FormControl isRequired>
						<FormLabel>Network</FormLabel>
						<SearchableSelect
							mode="single"
							options={options}
							value={networkName}
							onChange={setNetworkName}
							searchPlaceholder="Search networks…"
							emptyMessage="No networks to connect."
						/>
					</FormControl>
				</form>
			)}
		</Modal>
	);
}
