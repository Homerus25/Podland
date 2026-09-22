import { Button, VStack } from "@chakra-ui/react";
import type { ChangePasswordInput } from "@features/auth/services/authApi";
import {
	MutationDialog,
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
// obvious mistakes never reach the API. Skeleton invariants (reset on open,
// close on success, in-modal error region) come from MutationDialog.
export function ChangePasswordDialog({
	mutation,
	onClose,
	onSuccess,
}: ChangePasswordDialogProps) {
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting: isSubmittingForm, isValid },
	} = useForm<ChangePasswordFormValues>({
		defaultValues: { currentPassword: "", newPassword: "" },
		mode: "onChange",
	});

	const submit = handleSubmit((values) => {
		mutation.mutate(values);
	});

	return (
		<MutationDialog
			mutation={mutation}
			title="Change password"
			onClose={onClose}
			onSuccess={onSuccess}
			body={
				<form onSubmit={submit} noValidate>
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
						<Button
							type="submit"
							colorScheme="blue"
							isLoading={isSubmittingForm || mutation.isPending}
							isDisabled={!isValid || isSubmittingForm || mutation.isPending}
							data-testid="change-password-submit"
						>
							Change password
						</Button>
					</VStack>
				</form>
			}
		/>
	);
}
