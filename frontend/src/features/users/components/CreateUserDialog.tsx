import {
	FormControl,
	FormErrorMessage,
	FormLabel,
	Input,
	Select,
	VStack,
} from "@chakra-ui/react";
import type { CreateUserInput } from "@features/users/services/usersApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { useForm } from "react-hook-form";

interface UserFormValues {
	username: string;
	password: string;
	role: Exclude<CreateUserInput["role"], undefined>;
}

interface CreateUserDialogProps {
	mutation: MutationSlice<CreateUserInput>;
	onClose: () => void;
}

// Add-user dialog: username, password, and the role select (defaults to
// "user" — promotion is the deliberate act). The username is trimmed
// before submit (the backend trims it too); the password is sent
// byte-identical, only its length is validated. This component is the form
// owner, so it mounts with the dialog (fields start empty on every open)
// and publishes its validity through the scope for the module footer's
// disabled-while-invalid-or-pending submit; failure renders in the module's
// error region, success closes through it.
export function CreateUserDialog({ mutation, onClose }: CreateUserDialogProps) {
	const { register, handleSubmit, formState } = useForm<UserFormValues>({
		defaultValues: { username: "", password: "", role: "user" },
		// onChange so the footer submit's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		mutation.mutate({
			username: values.username.trim(),
			password: values.password,
			role: values.role,
		});
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Add user"
				onClose={onClose}
				formId="create-user-form"
				submitLabel="Create user"
				submitTestId="user-form-submit"
				body={
					<form id="create-user-form" onSubmit={submit} noValidate>
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
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
