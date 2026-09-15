import type { LoginCredentials, LoginResponse } from "@shared/types/api";
import { bareApi } from "@shared/utils/api";

export const authApi = {
	login: async (credentials: LoginCredentials): Promise<LoginResponse> => {
		const response = await bareApi.post<LoginResponse>(
			"/auth/login",
			credentials,
		);
		return response.data;
	},

	// Exchanges the httpOnly refresh cookie for a new access JWT (plus a
	// rotated cookie set by the server). 401 means no valid session.
	refresh: async (): Promise<LoginResponse> => {
		const response = await bareApi.post<LoginResponse>("/auth/refresh");
		return response.data;
	},

	// Revokes the refresh session behind the httpOnly cookie; idempotent.
	logout: async (): Promise<void> => {
		await bareApi.post("/auth/logout");
	},
};
