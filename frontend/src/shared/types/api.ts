// Role of a Podland account (backend/internal/domain/users/dto.go): the
// auth responses carry it so the UI can gate admin surfaces. "admin" is
// granted to the first admin created through first-run setup.
export type UserRole = "user" | "admin";

// The password policy's floor (backend/internal/models MinPasswordLength,
// answered as 400 PASSWORD_TOO_SHORT). Every password-setting surface —
// self-service change, admin create, admin reset — mirrors it client-side
// so the obvious mistake never needs a round trip. One vocabulary: this
// constant is the only place the number lives in the frontend.
export const PASSWORD_MIN_LENGTH = 8;

export interface User {
	username: string;
	role: UserRole;
}

export interface LoginCredentials {
	username: string;
	password: string;
	// The remember-me opt-in of POST /api/auth/login
	// (doku/description_auth_api.md): true mints a persistent refresh
	// session; absent or false keeps today's browser-session cookie.
	rememberDevice?: boolean;
}

// Body of POST /api/auth/login and POST /api/auth/refresh (the refresh
// response matches the login body; the refresh token itself lives only
// in the rotated httpOnly cookie).
export interface LoginResponse {
	token: string;
	username: string;
	role: UserRole;
}

export type MachinePermission = "read" | "write" | "admin";

// Machine fields every machines endpoint returns — the bare shape the
// backend calls MachineResponse (Create and Update answer it).
export interface MachineResponse {
	id: string;
	name: string;
	description: string;
	hostname: string;
	port: number;
	sshUsername: string;
	sshKeyFingerprint: string;
	podmanSocketPath: string;
	ownerId: string;
	createdAt: string;
	updatedAt: string;
}

// Machine as the enrichment endpoints return it — GET /api/machines and
// GET /api/machines/{id} both enrich every response with the caller's
// own permission and ownership (backend/internal/domain/machines
// /service.go AccessibleMachineResponse), so gating reads the facts
// straight off the machine object.
export interface Machine extends MachineResponse {
	permission: MachinePermission;
	isOwner: boolean;
}

// Access grant as returned by GET /api/machines/{id}/access
// (backend/internal/domain/machines/handler.go AccessResponse). The API
// identifies users by id only.
export interface AccessGrant {
	userId: string;
	permission: MachinePermission;
	grantedAt: string;
}

// Backend error envelope (backend/internal/errors/handlers.go ErrorResponse)
export interface ApiErrorEnvelope {
	requestId: string;
	error: {
		message: string;
		code: string;
	};
}

// The display-ready form of an error: code + human-readable message.
export interface ApiError {
	code: string;
	message: string;
}
