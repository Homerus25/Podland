import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { useForm } from "react-hook-form";

interface RenameFormValues {
	name: string;
}

interface RenameContainerDialogProps {
	// The container's current name: the input's prefill and the
	// unchanged-case guard's reference.
	currentName: string;
	mutation: MutationSlice<string>;
	onClose: () => void;
}

// Rename-container dialog: one text input prefilled with the current
// name. The client guards only the two cases it can decide — an empty
// name and an unchanged one — and leaves the name's shape to the server,
// which owns podman's regex. The name is trimmed before submit (podman
// trims it too), so the guards read the trimmed value. This component is
// the form owner, so it mounts with the dialog (the field starts at the
// current name on every open) and publishes its validity through the
// scope for the module footer; failure renders in the module's error
// region, success closes through it.
export function RenameContainerDialog({
	currentName,
	mutation,
	onClose,
}: RenameContainerDialogProps) {
	const { register, handleSubmit, formState } = useForm<RenameFormValues>({
		defaultValues: { name: currentName },
		// onChange so the footer submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		mutation.mutate(values.name.trim());
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Rename container"
				onClose={onClose}
				formId="rename-container-form"
				submitLabel="Rename"
				submitTestId="rename-container-submit"
				body={
					<form id="rename-container-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<FormControl isInvalid={!!errors.name} isRequired>
								<FormLabel htmlFor="container-name">Name</FormLabel>
								<Input
									id="container-name"
									fontFamily="mono"
									{...register("name", {
										validate: (value) => {
											const name = value.trim();
											if (name.length === 0) return "Name is required";
											if (name === currentName) return "Name is unchanged";
											return true;
										},
									})}
								/>
								<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
							</FormControl>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
