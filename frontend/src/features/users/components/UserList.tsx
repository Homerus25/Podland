import {
	Button,
	FormControl,
	FormLabel,
	HStack,
	Select,
	Td,
	Tr,
	useToast,
	VStack,
} from "@chakra-ui/react";
import { UserForm } from "@features/users/components/UserForm";
import {
	useChangeUserRole,
	useCreateUser,
	useResetUserPassword,
	useUsers,
} from "@features/users/hooks/useUsers";
import type {
	ChangeUserRoleInput,
	UserSummary,
} from "@features/users/services/usersApi";
import { MutationDialog } from "@shared/components/MutationDialog";
import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { ResourceList } from "@shared/components/ResourceList";
import type { UserRole } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useForm } from "react-hook-form";

// One-select form for the change-role dialog. It mounts with the dialog
// (the host renders it conditionally), so the select starts at the user's
// current role on every open; there is nothing to validate, both options
// are legal roles and the self-lockout guard lives server-side.
function UserRoleForm({
	user,
	onSubmit,
	isSubmitting,
}: {
	user: UserSummary;
	onSubmit: (input: ChangeUserRoleInput) => void;
	isSubmitting: boolean;
}) {
	const [role, setRole] = useState<UserRole>(user.role ?? "user");

	return (
		<form
			onSubmit={(event) => {
				event.preventDefault();
				onSubmit({ id: user.id, role });
			}}
			noValidate
		>
			<VStack spacing={4} align="stretch">
				<FormControl>
					<FormLabel htmlFor="change-user-role">Role</FormLabel>
					<Select
						id="change-user-role"
						data-testid="change-role-select"
						value={role}
						onChange={(event) => setRole(event.target.value as UserRole)}
					>
						<option value="user">user</option>
						<option value="admin">admin</option>
					</Select>
				</FormControl>
				<Button
					type="submit"
					colorScheme="blue"
					isLoading={isSubmitting}
					data-testid="change-role-submit"
				>
					Change role
				</Button>
			</VStack>
		</form>
	);
}

// One-field form for the reset-password dialog. It mounts with the dialog
// (the host renders it conditionally), so the field starts empty on every
// open. Client-side validation mirrors the backend's password policy
// (PASSWORD_TOO_SHORT, 8 characters) so the obvious mistake never needs a
// round trip; the reset itself needs no current-password check — the caller
// is the admin, not the target.
interface UserPasswordFormValues {
	newPassword: string;
}

function UserPasswordForm({
	user,
	onSubmit,
	isSubmitting,
}: {
	user: UserSummary;
	onSubmit: (input: { id: string; newPassword: string }) => void;
	isSubmitting: boolean;
}) {
	const {
		register,
		handleSubmit,
		formState: { errors, isSubmitting: isSubmittingForm, isValid },
	} = useForm<UserPasswordFormValues>({
		defaultValues: { newPassword: "" },
		// onChange so the submit button's disabled state tracks validity as
		// the user types, not only after a failed submit attempt.
		mode: "onChange",
	});

	return (
		<form
			onSubmit={handleSubmit((values) =>
				onSubmit({ id: user.id, newPassword: values.newPassword }),
			)}
			noValidate
		>
			<VStack spacing={4} align="stretch">
				<PasswordField
					id="user-new-password"
					label="New password"
					mono
					registration={register(
						"newPassword",
						passwordRules("Password is required"),
					)}
					error={errors.newPassword?.message}
				/>
				<Button
					type="submit"
					colorScheme="blue"
					isLoading={isSubmittingForm || isSubmitting}
					isDisabled={!isValid || isSubmittingForm || isSubmitting}
					data-testid="reset-password-submit"
				>
					Reset password
				</Button>
			</VStack>
		</form>
	);
}

// Each row owns its change-role and reset-password mutation instances, so
// pending state and errors stay scoped to this user's dialogs and other
// rows stay usable. The shell, reset lifecycle, close-on-success, and error
// region are MutationDialog's. The self-role rejection (400 SELF_ROLE_CHANGE)
// and reset failures surface through the default error region with the
// backend's message; a successful reset toasts the forced re-login, since
// the backend revoked every session of the target user.
function UserRow({ user }: { user: UserSummary }) {
	const changeRole = useChangeUserRole();
	const resetPassword = useResetUserPassword();
	const toast = useToast();
	const [isRoleOpen, setIsRoleOpen] = useState(false);
	const [isPasswordOpen, setIsPasswordOpen] = useState(false);

	return (
		<>
			{/* The row testid carries the (unique) username so tests select
			    one user's row with within() instead of DOM walking. */}
			<Tr data-testid={`user-row-${user.username}`}>
				<Td fontFamily="mono">{user.username}</Td>
				<Td data-testid="user-role">{user.role ?? "—"}</Td>
				<Td>{user.createdAt ? formatRelativeDate(user.createdAt) : "—"}</Td>
				<Td>
					<HStack spacing={2}>
						<Button
							size="xs"
							variant="outline"
							onClick={() => setIsRoleOpen(true)}
							data-testid="change-role-button"
						>
							Change role
						</Button>
						<Button
							size="xs"
							variant="outline"
							onClick={() => setIsPasswordOpen(true)}
							data-testid="reset-password-button"
						>
							Reset password
						</Button>
					</HStack>
				</Td>
			</Tr>
			{isRoleOpen ? (
				<MutationDialog
					mutation={changeRole}
					title={`Change role — ${user.username}`}
					onClose={() => setIsRoleOpen(false)}
					body={
						<UserRoleForm
							user={user}
							onSubmit={(input) => changeRole.mutate(input)}
							isSubmitting={changeRole.isPending}
						/>
					}
				/>
			) : null}
			{isPasswordOpen ? (
				<MutationDialog
					mutation={resetPassword}
					title={`Reset password — ${user.username}`}
					onClose={() => setIsPasswordOpen(false)}
					onSuccess={() =>
						toast({
							title: `Password reset: ${user.username} must sign in with the new password`,
							status: "success",
							isClosable: true,
						})
					}
					body={
						<UserPasswordForm
							user={user}
							onSubmit={(input) => resetPassword.mutate(input)}
							isSubmitting={resetPassword.isPending}
						/>
					}
				/>
			) : null}
		</>
	);
}

// The admin Users page body. The list rides the shared users query — the
// same directory the share form's picker reads — and the toolbar's add-user
// dialog rides MutationDialog: it owns the shell, the reset lifecycle,
// close-on-success, and the in-modal error region where creation failures
// (409 USERNAME_TAKEN, PASSWORD_TOO_SHORT) surface. This adapter owns only
// the columns, the rows (each with its change-role and reset-password
// dialogs), and the form wiring. The page is admin-gated at the route, so
// the action column is unconditional.
export function UserList() {
	const { data: users, isPending, error } = useUsers();
	const createUser = useCreateUser();
	const [isCreateOpen, setIsCreateOpen] = useState(false);

	return (
		<>
			<ResourceList
				resource="user"
				isPending={isPending}
				error={error}
				items={users}
				columns={[
					{ label: "Username" },
					{ label: "Role" },
					{ label: "Created" },
					{},
				]}
				emptyMessage="No users yet."
				renderRow={(user) => <UserRow key={user.id} user={user} />}
				toolbar={
					<Button
						size="sm"
						colorScheme="blue"
						onClick={() => setIsCreateOpen(true)}
						data-testid="add-user-button"
					>
						Add user
					</Button>
				}
			/>
			{isCreateOpen ? (
				<MutationDialog
					mutation={createUser}
					title="Add user"
					onClose={() => setIsCreateOpen(false)}
					body={
						<UserForm
							onSubmit={(input) => createUser.mutate(input)}
							isSubmitting={createUser.isPending}
						/>
					}
				/>
			) : null}
		</>
	);
}
