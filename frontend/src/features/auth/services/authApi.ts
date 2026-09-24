import type {
	LoginCredentials,
	LoginResponse,
	UserRole,
} from "@shared/types/api";
import { bareApi, getBearerToken } from "@shared/utils/api";

// Body of POST /api/auth/password (doku/description_auth_api.md).
export interface ChangePasswordInput {
	currentPassword: string;
	newPassword: string;
}

// Response of GET /api/auth/status (doku/description_auth_api.md). The
// anonymous shape is { authenticated: false, needsSetup }; an
// authenticated caller gets userId and role and always needsSetup false.
export interface AuthStatusResponse {
	authenticated: boolean;
	needsSetup: boolean;
	userId?: string;
	role?: UserRole;
}

// Body of POST /api/setup/user: the credentials of the instance's first
// user — always created with the admin role.
export interface SetupInput {
	username: string;
	password: string;
}

export const authApi = {
	// The anonymous boot probe. It rides the bare transport like the other
	// auth calls: the probe is anonymous by construction, and its failure
	// is a logged-out boot on an unreachable instance — normal login —
	// never session death.
	status: async (): Promise<AuthStatusResponse> => {
		const response = await bareApi.get<AuthStatusResponse>("/auth/status");
		return response.data;
	},

	// First-run setup: creates the first user (admin) and answers with the
	// login payload — access token in the body, refresh cookie set — so
	// the caller is signed in on the spot. 409 SETUP_COMPLETED means the
	// instance was configured meanwhile.
	setup: async (input: SetupInput): Promise<LoginResponse> => {
		const response = await bareApi.post<LoginResponse>("/setup/user", input);
		return response.data;
	},
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
