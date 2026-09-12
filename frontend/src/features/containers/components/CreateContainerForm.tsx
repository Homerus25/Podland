import {
	Button,
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	VStack,
} from "@chakra-ui/react";
import type { CreateContainerInput } from "@features/containers/services/containersApi";
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

interface CreateContainerFormProps {
	// Fire-and-forget: the host dialog's MutationDialog closes on success and
	// renders the failure, so the form only forwards the payload.
	onSubmit: (input: CreateContainerInput) => void;
	isSubmitting: boolean;
}

export function CreateContainerForm({
	onSubmit,
	isSubmitting,
}: CreateContainerFormProps) {
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting: isSubmittingForm },
	} = useForm<ContainerFormValues>({
		defaultValues: { image: "", name: "", command: "" },
	});

	const submit = handleSubmit((values) => {
		const command = parseCommand(values.command);
		onSubmit({
			image: values.image.trim(),
			...(values.name.trim() ? { name: values.name.trim() } : {}),
			...(command ? { command } : {}),
		});
	});

	return (
		<form onSubmit={submit} noValidate>
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
					<FormLabel htmlFor="container-command">Command (optional)</FormLabel>
					<Input
						id="container-command"
						placeholder="redis-server --appendonly yes"
						{...register("command")}
					/>
					<FormErrorMessage>{errors.command?.message}</FormErrorMessage>
				</FormControl>
				<Button
					type="submit"
					colorScheme="blue"
					isLoading={isSubmittingForm || isSubmitting}
					data-testid="container-form-submit"
				>
					Create container
				</Button>
			</VStack>
		</form>
	);
}
