import { FormControl, FormErrorMessage, FormLabel } from "@chakra-ui/react";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import { useContainers } from "@features/containers/hooks/useContainers";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { runningContainers } from "@features/networks/components/containerOptions";
import {
	useConnectContainer,
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
	pick: string;
}

// The fixed side discriminates the flow: the Networks tab passes the
// network and the dialog picks the container; the Containers tab passes
// the container and the dialog picks the network.
type ConnectSide =
	| { kind: "network"; network: NetworkSummary }
	| { kind: "container"; container: ContainerSummary };

type ConnectDialogProps = {
	machineId: string;
	onClose: () => void;
} & (
	| { network: NetworkSummary; container?: never }
	| { container: ContainerSummary; network?: never }
);

// The one owner of the connect ritual (issue 274): both tabs' connect
// flows ride this dialog, entered with the fixed side and a picker for
// the other. The dialog fetches its own options and drives the connect
// mutation itself, including the both-lists invalidation that rides the
// mutation. Options that can only produce an error never appear: the
// container side offers running containers only, already-attached ones
// excluded; the network side offers the networks the container is not on.
// The chosen side is a required form field, so the module footer's submit
// stays disabled until one is picked; confirm sends { networkName,
// containerId }, the shape both connect surfaces share. Failure renders in
// the module's error region and keeps the dialog open, success closes
// through it.
export function ConnectDialog(props: ConnectDialogProps) {
	const { machineId, onClose } = props;
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
					label: "Container",
					requiredMessage: "Container is required",
					searchPlaceholder: "Search containers…",
					emptyMessage: "No running containers to connect.",
				}
			: {
					title: `Connect ${containerDisplayName(side.container)} to a network`,
					formId: "connect-network-form",
					submitTestId: "connect-network-submit",
					label: "Network",
					requiredMessage: "Network is required",
					searchPlaceholder: "Search networks…",
					emptyMessage: "No networks to connect.",
				};

	const { data: containers } = useContainers(machineId);
	const { data: networks } = useNetworks(machineId);
	const connect = useConnectContainer(machineId);

	const { register, handleSubmit, setValue, watch, formState } =
		useForm<ConnectFormValues>({
			defaultValues: { pick: "" },
			// onChange so the footer submit's disabled state tracks validity
			// as the user picks, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;
	const selected = watch("pick");
	// The select is not a native input, so the field registers without
	// props; setValue below drives it and shouldValidate keeps the footer's
	// submit policy in step with the pick.
	register("pick", { required: copy.requiredMessage });

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

	const submit = handleSubmit((values) => {
		connect.mutate(
			side.kind === "network"
				? { networkName: side.network.name, containerId: values.pick }
				: { networkName: values.pick, containerId: side.container.id },
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
				body={
					<form id={copy.formId} onSubmit={submit} noValidate>
						<FormControl isInvalid={!!errors.pick} isRequired>
							<FormLabel>{copy.label}</FormLabel>
							<SearchableSelect
								mode="single"
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
