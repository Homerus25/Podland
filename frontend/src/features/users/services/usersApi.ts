import type { UserRole } from "@shared/types/api";
import { api, bareApi, getBearerToken } from "@shared/utils/api";

// User as returned by GET /api/users. The shape depends on the caller's
// role: every authenticated caller receives the minimal {id, username}
// (backend UserSummaryResponse), admins additionally receive role and
// creation time (backend UserResponse) — so the admin-only fields are
// optional here.
export interface UserSummary {
	id: string;
	username: string;
	role?: UserRole;
	createdAt?: string;
}

// Lists the Podland users, ordered by username. Available to any
// authenticated caller; machine owners use it to resolve grant user ids
// to usernames when sharing a machine.
export async function listUsers(): Promise<UserSummary[]> {
	const response = await api.get<UserSummary[]>("/users");
	return response.data;
}

// Body of POST /api/users (doku/description_users_api.md). Admin-only;
// the role is optional and defaults to "user" server-side.
export interface CreateUserInput {
	username: string;
	password: string;
	role?: UserRole;
}

// Creates a user under the password policy (minimum 8 characters) and
// returns the created record. 409 USERNAME_TAKEN on a duplicate username.
export async function createUser(input: CreateUserInput): Promise<UserSummary> {
	const response = await api.post<UserSummary>("/users", input);
	return response.data;
}

// Body of PUT /api/users/{id}/role (doku/description_users_api.md). The
// backend's self-lockout guard rejects a caller changing their own role
// (400 SELF_ROLE_CHANGE, "Cannot change your own role").
export interface ChangeUserRoleInput {
	id: string;
	role: UserRole;
}

// Changes a user's role and returns the updated record. Admin-only; the
// change takes effect immediately for the target's existing tokens.
export async function changeUserRole(
	input: ChangeUserRoleInput,
): Promise<UserSummary> {
	const response = await api.put<UserSummary>(`/users/${input.id}/role`, {
		role: input.role,
	});
	return response.data;
}

// Body of PUT /api/users/{id}/password (doku/description_users_api.md).
// Admin reset with re-authentication: adminPassword is the acting admin's
// own password (verified against the caller, not the target), newPassword
// must satisfy the 8-character minimum enforced server-side.
export interface ResetUserPasswordInput {
	id: string;
	adminPassword: string;
	newPassword: string;
}

// Resets a user's password and returns the updated record. Admin-only;
// every session of the target user is revoked, so the target must log in
// again with the new password. Authenticated (the bearer rides along from
// the shared credential holder), but its 401s are input errors — a wrong
// adminPassword answers "Current password is incorrect", not session
// death — so it rides the bare transport like the change-password call:
// no refresh/replay ritual.
export async function resetUserPassword(
	input: ResetUserPasswordInput,
): Promise<UserSummary> {
	const response = await bareApi.put<UserSummary>(
		`/users/${input.id}/password`,
		{ adminPassword: input.adminPassword, newPassword: input.newPassword },
		{ headers: { Authorization: `Bearer ${getBearerToken()}` } },
	);
	return response.data;
}
