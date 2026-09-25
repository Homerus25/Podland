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
import { CreateUserDialog } from "@features/users/components/CreateUserDialog";
import {
	useChangeUserRole,
	useCreateUser,
	useResetUserPassword,
	useUsers,
} from "@features/users/hooks/useUsers";
import type {
	ChangeUserRoleInput,
	ResetUserPasswordInput,
	UserSummary,
} from "@features/users/services/usersApi";
import {
	FormDialog,
	FormValidityScope,
	type MutationSlice,
} from "@shared/components/MutationDialog";
import { PasswordField, passwordRules } from "@shared/components/PasswordField";
import { ResourceList } from "@shared/components/ResourceList";
import type { UserRole } from "@shared/types/api";
import { formatRelativeDate } from "@shared/utils/format";
import { useState } from "react";
import { useForm } from "react-hook-form";

// The change-role dialog: one select, the only dialog form that publishes
// no scope — there is nothing to validate, both options are legal roles
// and the self-lockout guard lives server-side, so the footer's submit
// gates on pending alone. It mounts with the dialog (the host renders it
// conditionally), so the select starts at the user's current role on
// every open.
function ChangeUserRoleDialog({
	user,
	mutation,
	onClose,
}: {
	user: UserSummary;
	mutation: MutationSlice<ChangeUserRoleInput>;
	onClose: () => void;
}) {
	const [role, setRole] = useState<UserRole>(user.role ?? "user");

	return (
		<FormDialog
			mutation={mutation}
			title={`Change role — ${user.username}`}
			onClose={onClose}
			formId="change-role-form"
			submitLabel="Change role"
			submitTestId="change-role-submit"
			body={
				<form
					id="change-role-form"
					onSubmit={(event) => {
						event.preventDefault();
						mutation.mutate({ id: user.id, role });
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
					</VStack>
				</form>
			}
		/>
	);
}

// Reset-password dialog: two fields — the acting admin's own password
// above the target's new one — the change-password dialog's order. This
// component is the form owner, so it mounts with the dialog (the host
// renders it conditionally) and the fields start empty on every open; it
// publishes its validity through the scope for the module footer. The
// length policy applies to the new password only; the admin's own password
// is just required, and a wrong value comes back as an ordinary in-dialog
// 401.
interface UserPasswordFormValues {
	adminPassword: string;
	newPassword: string;
}

function ResetUserPasswordDialog({
	user,
	mutation,
	onClose,
	onSuccess,
}: {
	user: UserSummary;
	mutation: MutationSlice<ResetUserPasswordInput>;
	onClose: () => void;
	onSuccess?: () => void;
}) {
	const { register, handleSubmit, formState } = useForm<UserPasswordFormValues>(
		{
			defaultValues: { adminPassword: "", newPassword: "" },
			// onChange so the footer submit's disabled state tracks validity as
			// the user types, not only after a failed submit attempt.
			mode: "onChange",
		},
	);
	const { errors } = formState;

	return (
		<FormValidityScope formState={formState}>
			<FormDialog
				mutation={mutation}
				title={`Reset password — ${user.username}`}
				onClose={onClose}
				onSuccess={onSuccess}
				formId="reset-password-form"
				submitLabel="Reset password"
				submitTestId="reset-password-submit"
				body={
					<form
						id="reset-password-form"
						onSubmit={handleSubmit((values) =>
							mutation.mutate({
								id: user.id,
								adminPassword: values.adminPassword,
								newPassword: values.newPassword,
							}),
						)}
						noValidate
					>
						<VStack spacing={4} align="stretch">
							<PasswordField
								id="user-admin-password"
								label="Your password"
								autoComplete="current-password"
								registration={register("adminPassword", {
									required: "Your password is required",
								})}
								error={errors.adminPassword?.message}
							/>
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
						</VStack>
					</form>
				}
			/>
		</FormValidityScope>
	);
}

// Each row owns its change-role and reset-password mutation instances, so
// pending state and errors stay scoped to this user's dialogs and other
// rows stay usable. Both dialogs ride the module's FormDialog preset
// (footer Cancel + submit, close-on-success, error region). The self-role
// rejection (400 SELF_ROLE_CHANGE) and reset failures surface through the
// default error region with the backend's message; a successful reset
// toasts the forced re-login, since the backend revoked every session of
// the target user.
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
				<ChangeUserRoleDialog
					user={user}
					mutation={changeRole}
					onClose={() => setIsRoleOpen(false)}
				/>
			) : null}
			{isPasswordOpen ? (
				<ResetUserPasswordDialog
					user={user}
					mutation={resetPassword}
					onClose={() => setIsPasswordOpen(false)}
					onSuccess={() =>
						toast({
							title: `Password reset: ${user.username} must sign in with the new password`,
							status: "success",
							isClosable: true,
						})
					}
				/>
			) : null}
		</>
	);
}

// The admin Users page body. The list rides the shared users query — the
// same directory the share form's picker reads — and the toolbar's add-user
// dialog rides the module's FormDialog preset: it owns the shell, the
// reset lifecycle, close-on-success, and the in-modal error region where
// creation failures (409 USERNAME_TAKEN, PASSWORD_TOO_SHORT) surface. This
// adapter owns only the columns, the rows (each with its change-role and
// reset-password dialogs), and the form wiring. The page is admin-gated at
// the route, so the action column is unconditional.
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
				<CreateUserDialog
					mutation={createUser}
					onClose={() => setIsCreateOpen(false)}
				/>
			) : null}
		</>
	);
}
