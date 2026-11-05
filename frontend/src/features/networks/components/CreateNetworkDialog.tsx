import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import { containerDisplayName } from "@features/containers/components/containerDisplayName";
import { useContainers } from "@features/containers/hooks/useContainers";
import { runningContainers } from "@features/networks/components/containerOptions";
import type { CreateNetworkInput } from "@features/networks/services/networksApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { SearchableSelect } from "@shared/components/SearchableSelect";
import { useState } from "react";
import { useForm } from "react-hook-form";

interface NetworkFormValues {
	name: string;
}

interface CreateNetworkDialogProps {
	hostId: string;
	mutation: MutationSlice<CreateNetworkInput>;
	onClose: () => void;
}

// Create-network dialog: a required name and an optional multi-select of
// the host's running containers to attach in the same call. This
// component is the form owner, so it mounts with the dialog (fields start
// empty on every open) and publishes its validity through the scope for the
// module footer; a failure (409 duplicate, attach failure naming the
// container) renders in the module's error region and keeps the dialog
// open, success closes through it.
export function CreateNetworkDialog({
	hostId,
	mutation,
	onClose,
}: CreateNetworkDialogProps) {
	const { data: containers } = useContainers(hostId);
	const [selected, setSelected] = useState<string[]>([]);
	const { register, handleSubmit, formState } = useForm<NetworkFormValues>({
		defaultValues: { name: "" },
		// onChange so the footer submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		mutation.mutate({
			name: values.name.trim(),
			...(selected.length ? { containers: selected } : {}),
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create network"
				onClose={onClose}
				formId="create-network-form"
				submitLabel="Create network"
				submitTestId="create-network-submit"
				body={
					<form id="create-network-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<FormControl isInvalid={!!errors.name} isRequired>
								<FormLabel htmlFor="network-name">Name</FormLabel>
								<Input
									id="network-name"
									placeholder="app-net"
									{...register("name", {
										required: "Name is required",
										validate: (value) =>
											value.trim().length > 0 || "Name is required",
									})}
								/>
								<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
							</FormControl>
							<FormControl>
								<FormLabel>Containers (optional)</FormLabel>
								<SearchableSelect
									mode="multi"
									options={runningContainers(containers).map((container) => ({
										value: container.id,
										label: containerDisplayName(container),
									}))}
									value={selected}
									onChange={setSelected}
									searchPlaceholder="Search containers…"
									emptyMessage="No running containers."
								/>
							</FormControl>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
