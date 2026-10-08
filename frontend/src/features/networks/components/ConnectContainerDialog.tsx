import { FormControl, FormErrorMessage, FormLabel } from "@chakra-ui/react";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import { useContainers } from "@features/containers/hooks/useContainers";
import { runningContainers } from "@features/networks/components/containerOptions";
import type {
	ConnectContainerVariables,
	NetworkSummary,
} from "@features/networks/services/networksApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import { useForm } from "react-hook-form";

interface ConnectFormValues {
	container: string;
}

interface ConnectContainerDialogProps {
	machineId: string;
	network: NetworkSummary;
	mutation: MutationSlice<ConnectContainerVariables>;
	onClose: () => void;
}

// Connect-container dialog: a single-select of the machine's running
// containers, already-attached ones excluded — an entry that can only
// produce an error is bait. The chosen container is a required form field,
// so the module footer's submit stays disabled until one is picked; confirm
// sends { networkName, containerId }, the shape both connect surfaces share.
// Failure renders in the module's error region and keeps the dialog open,
// success closes through it.
export function ConnectContainerDialog({
	machineId,
	network,
	mutation,
	onClose,
}: ConnectContainerDialogProps) {
	const { data: containers } = useContainers(machineId);
	const { register, handleSubmit, setValue, watch, formState } =
		useForm<ConnectFormValues>({
			defaultValues: { container: "" },
			// onChange so the footer submit's disabled state tracks validity
			// as the user picks, not only after a failed submit attempt.
			mode: "onChange",
		});
	const { errors } = formState;
	const selected = watch("container");
	// The select is not a native input, so the field registers without
	// props; setValue below drives it and shouldValidate keeps the footer's
	// submit policy in step with the pick.
	register("container", { required: "Container is required" });

	// Already-attached = the container's networks include this network.
	const options = runningContainers(containers)
		.filter((container) => !(container.networks ?? []).includes(network.name))
		.map((container) => ({
			value: container.id,
			label: containerDisplayName(container),
		}));

	const submit = handleSubmit((values) => {
		mutation.mutate({
			networkName: network.name,
			containerId: values.container,
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title={`Connect container to ${network.name}`}
				onClose={onClose}
				formId="connect-container-form"
				submitLabel="Connect"
				submitTestId="connect-container-submit"
				body={
					<form id="connect-container-form" onSubmit={submit} noValidate>
						<FormControl isInvalid={!!errors.container} isRequired>
							<FormLabel>Container</FormLabel>
							<SearchableSelect
								mode="single"
								options={options}
								value={selected}
								onChange={(value) =>
									setValue("container", value, { shouldValidate: true })
								}
								searchPlaceholder="Search containers…"
								emptyMessage="No running containers to connect."
							/>
							<FormErrorMessage>{errors.container?.message}</FormErrorMessage>
						</FormControl>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
