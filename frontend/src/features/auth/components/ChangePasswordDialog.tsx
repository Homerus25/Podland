import { VStack } from "@chakra-ui/react";
import type { ChangePasswordInput } from "@features/auth/services/authApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { useForm } from "react-hook-form";

interface ChangePasswordFormValues {
	currentPassword: string;
	newPassword: string;
}

interface ChangePasswordDialogProps {
	mutation: MutationSlice<ChangePasswordInput>;
	onClose: () => void;
	// Fired once on success, before the module closes: the host adopts the
	// re-issued session from the mutation result here.
	onSuccess?: () => void;
}

// Self-service password change. The current password proves authority, the
// new one must satisfy the length policy; both are checked client-side so
// obvious mistakes never reach the API. The skeleton invariants (reset on
// open, close on success, in-modal error region, the footer's ghost Cancel
// and disabled-while-invalid-or-pending submit) come from the module: this
// form owner publishes its validity through the scope sitting above the
// dialog, which is where the footer reads it through the portal.
export function ChangePasswordDialog({
	mutation,
	onClose,
	onSuccess,
}: ChangePasswordDialogProps) {
	const { register, handleSubmit, formState } =
		useForm<ChangePasswordFormValues>({
			defaultValues: { currentPassword: "", newPassword: "" },
			mode: "onChange",
		});
	const { errors } = formState;

	const submit = handleSubmit((values) => {
		mutation.mutate(values);
	});

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title="Change password"
				onClose={onClose}
				onSuccess={onSuccess}
				formId="change-password-form"
				submitLabel="Change password"
				submitTestId="change-password-submit"
				body={
					<form id="change-password-form" onSubmit={submit} noValidate>
						<VStack spacing={4} align="stretch">
							<PasswordField
								id="current-password"
								label="Current password"
								autoComplete="current-password"
								registration={register("currentPassword", {
									required: "Current password is required",
								})}
								error={errors.currentPassword?.message}
							/>
							<PasswordField
								id="new-password"
								label="New password"
								registration={register(
									"newPassword",
									passwordRules("New password is required"),
								)}
								error={errors.newPassword?.message}
							/>
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}
