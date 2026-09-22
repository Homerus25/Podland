import {
	Button,
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	Select,
	VStack,
} from "@chakra-ui/react";
import type { CreateUserInput } from "@features/users/services/usersApi";
import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { useForm } from "react-hook-form";

interface UserFormValues {
	username: string;
	password: string;
	role: Exclude<CreateUserInput["role"], undefined>;
}

interface UserFormProps {
	// Fire-and-forget: the host dialog's MutationDialog closes on success and
	// renders the failure, so the form only forwards the payload. The select
	// always carries a role, so the payload's optional role is always sent.
	onSubmit: (input: CreateUserInput) => void;
	isSubmitting: boolean;
}

// Add-user form: username, password, and the role select (defaults to
// "user" — promotion is the deliberate act). The username is trimmed
// before submit (the backend trims it too); the password is sent
// byte-identical, only its length is validated.
export function UserForm({ onSubmit, isSubmitting }: UserFormProps) {
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting: isSubmittingForm, isValid },
	} = useForm<UserFormValues>({
		defaultValues: { username: "", password: "", role: "user" },
		// onChange so the submit button's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});

	const submit = handleSubmit((values) => {
		onSubmit({
			username: values.username.trim(),
			password: values.password,
			role: values.role,
		});
	});

	return (
		<form onSubmit={submit} noValidate>
			<VStack spacing={4} align="stretch">
				<FormControl isInvalid={!!errors.username} isRequired>
					<FormLabel htmlFor="user-username">Username</FormLabel>
					<Input
						id="user-username"
						fontFamily="mono"
						autoComplete="off"
						{...register("username", {
							required: "Username is required",
							validate: (value) =>
								value.trim().length > 0 || "Username is required",
						})}
					/>
					<FormErrorMessage>{errors.username?.message}</FormErrorMessage>
				</FormControl>
				<PasswordField
					id="user-password"
					label="Password"
					mono
					registration={register(
						"password",
						passwordRules("Password is required"),
					)}
					error={errors.password?.message}
				/>
				<FormControl>
					<FormLabel htmlFor="user-role">Role</FormLabel>
					<Select
						id="user-role"
						data-testid="user-role-select"
						{...register("role")}
					>
						<option value="user">user</option>
						<option value="admin">admin</option>
					</Select>
				</FormControl>
				<Button
					type="submit"
					colorScheme="blue"
					isLoading={isSubmittingForm || isSubmitting}
					isDisabled={!isValid || isSubmittingForm || isSubmitting}
					data-testid="user-form-submit"
				>
					Create user
				</Button>
			</VStack>
		</form>
	);
}
