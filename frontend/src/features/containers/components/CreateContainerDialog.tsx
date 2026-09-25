import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import type { CreateContainerInput } from "@features/containers/services/containersApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { useForm } from "react-hook-form";

interface ContainerFormValues {
	image: string;
	name: string;
	command: string;
}

// The backend takes the command as a string array; the form offers one text
// input and splits it on whitespace ("redis-server --appendonly yes").
function parseCommand(text: string): string[] | undefined {
	const trimmed = text.trim();
	return trimmed ? trimmed.split(/\s+/) : undefined;
}

interface CreateContainerDialogProps {
	mutation: MutationSlice<CreateContainerInput>;
	onClose: () => void;
}

// Create-container dialog: required image, optional name and command. This
// component is the form owner, so it mounts with the dialog (fields start
// empty on every open) and publishes its validity through the scope for the
// module footer; failure renders in the module's error region, success
// closes through it.
export function CreateContainerDialog({
	mutation,
	onClose,
}: CreateContainerDialogProps) {
	const { register, handleSubmit, formState } = useForm<ContainerFormValues>({
		defaultValues: { image: "", name: "", command: "" },
		// onChange so the footer submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		const command = parseCommand(values.command);
		mutation.mutate({
			image: values.image.trim(),
			...(values.name.trim() ? { name: values.name.trim() } : {}),
			...(command ? { command } : {}),
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Create container"
				onClose={onClose}
				formId="create-container-form"
				submitLabel="Create container"
				submitTestId="container-form-submit"
				body={
					<form id="create-container-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<FormControl isInvalid={!!errors.image} isRequired>
								<FormLabel htmlFor="container-image">Image</FormLabel>
								<Input
									id="container-image"
									placeholder="docker.io/library/nginx:latest"
									{...register("image", {
										required: "Image is required",
										validate: (value) =>
											value.trim().length > 0 || "Image is required",
									})}
								/>
								<FormErrorMessage>{errors.image?.message}</FormErrorMessage>
							</FormControl>
							<FormControl isInvalid={!!errors.name}>
								<FormLabel htmlFor="container-name">Name (optional)</FormLabel>
								<Input id="container-name" {...register("name")} />
								<FormErrorMessage>{errors.name?.message}</FormErrorMessage>
							</FormControl>
							<FormControl isInvalid={!!errors.command}>
								<FormLabel htmlFor="container-command">
									Command (optional)
								</FormLabel>
								<Input
									id="container-command"
									placeholder="redis-server --appendonly yes"
									{...register("command")}
								/>
								<FormErrorMessage>{errors.command?.message}</FormErrorMessage>
							</FormControl>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
