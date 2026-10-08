import { FormControl, FormErrorMessage, FormLabel } from "@chakra-ui/react";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import type { ContainerSummary } from "@features/containers/services/containersApi";
import { useNetworks } from "@features/networks/hooks/useNetworks";
import type { ConnectContainerVariables } from "@features/networks/services/networksApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import { useForm } from "react-hook-form";

interface ConnectNetworkFormValues {
	network: string;
}

interface ConnectNetworkDialogProps {
	machineId: string;
	container: ContainerSummary;
	mutation: MutationSlice<ConnectContainerVariables>;
	onClose: () => void;
}

// Connect-to-network dialog (the Containers tab's half of issue 256): a
// single-select of the machine's networks, the container's current ones
// excluded — an entry that can only produce an error is bait. The chosen
// network is a required form field, so the module footer's submit stays
// disabled until one is picked; confirm sends { networkName, containerId },
// the shape both connect surfaces share. Failure renders in the module's
// error region and keeps the dialog open, success closes through it.
export function ConnectNetworkDialog({
	machineId,
	container,
	mutation,
	onClose,
}: ConnectNetworkDialogProps) {
	const { data: networks } = useNetworks(machineId);
	const { register, handleSubmit, setValue, watch, formState } =
		useForm<ConnectNetworkFormValues>({
			defaultValues: { network: "" },
			// onChange so the footer submit's disabled state tracks validity
			// as the user picks, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;
	const selected = watch("network");
	// The select is not a native input, so the field registers without
	// props; setValue below drives it and shouldValidate keeps the footer's
	// submit policy in step with the pick.
	register("network", { required: "Network is required" });

	// The container's current networks are excluded: connecting it to one
	// of them can only fail.
	const options = (networks ?? [])
		.filter((network) => !(container.networks ?? []).includes(network.name))
		.map((network) => ({ value: network.name, label: network.name }));

	const submit = handleSubmit((values) => {
		mutation.mutate({
			networkName: values.network,
			containerId: container.id,
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title={`Connect ${containerDisplayName(container)} to a network`}
				onClose={onClose}
				formId="connect-network-form"
				submitLabel="Connect"
				submitTestId="connect-network-submit"
				body={
					<form id="connect-network-form" onSubmit={submit} noValidate>
						<FormControl isInvalid={!!errors.network} isRequired>
							<FormLabel>Network</FormLabel>
							<SearchableSelect
								mode="single"
								options={options}
								value={selected}
								onChange={(value) =>
									setValue("network", value, { shouldValidate: true })
								}
								searchPlaceholder="Search networks…"
								emptyMessage="No networks to connect."
							/>
							<FormErrorMessage>{errors.network?.message}</FormErrorMessage>
						</FormControl>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
