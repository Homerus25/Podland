import type { LoginCredentials, LoginResponse } from "@shared/types/api";
import { bareApi, getBearerToken } from "@shared/utils/api";

// Body of POST /api/auth/password (doku/description_auth_api.md).
export interface ChangePasswordInput {
	currentPassword: string;
	newPassword: string;
}

export const authApi = {
	login: async (credentials: LoginCredentials): Promise<LoginResponse> => {
		const response = await bareApi.post<LoginResponse>(
			"/auth/login",
			credentials,
		);
		return response.data;
	},

	// Self-service password change. Authenticated (the bearer rides along
	// from the shared credential holder), but its 401s are input errors —
	// "Current password is incorrect" — not session death, so it rides the
	// bare transport like the other auth calls: no refresh/replay ritual.
	changePassword: async (
		input: ChangePasswordInput,
	): Promise<LoginResponse> => {
		const response = await bareApi.post<LoginResponse>(
			"/auth/password",
			input,
			{ headers: { Authorization: `Bearer ${getBearerToken()}` } },
		);
		return response.data;
	},

	// Revokes the refresh session behind the httpOnly cookie; idempotent.
	logout: async (): Promise<void> => {
		await bareApi.post("/auth/logout");
	},
};
