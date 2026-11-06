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

export type HostPermission = "read" | "write" | "admin";

// Host fields every hosts endpoint returns — the bare shape the
// backend calls HostResponse (Create and Update answer it).
export interface HostResponse {
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

// Host as the enrichment endpoints return it — GET /api/hosts and
// GET /api/hosts/{id} both enrich every response with the caller's
// own permission and ownership (backend/internal/domain/hosts
// /service.go AccessibleHostResponse), so gating reads the facts
// straight off the host object.
export interface Host extends HostResponse {
	permission: HostPermission;
	isOwner: boolean;
}

// Access grant as returned by GET /api/hosts/{id}/access
// (backend/internal/domain/hosts/handler.go AccessResponse). The API
// identifies users by id only.
export interface AccessGrant {
	userId: string;
	permission: HostPermission;
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

// One entry of a bulk-remove report (backend/internal/bulkremove Entry):
// the requested id — an id for containers, images, pods and quadlets, a
// name for volumes and networks — the outcome, and, on a failure, the code
// and message the client shows. Every resource's
// POST /api/hosts/{id}/<domain>/remove answers one entry per requested id,
// in request order, always 200: a per-item failure is an entry, never a
// status code. Shared here rather than colocated in one feature's services
// module because all seven host-scoped resources answer the same shape.
export interface BulkRemoveEntry {
	id: string;
	success: boolean;
	error?: ApiError;
}

// The request body every resource's POST /api/hosts/{id}/<domain>/remove
// takes (backend/internal/bulkremove Request): the ids to remove — an id for
// containers, images, pods and quadlets, a name for volumes and networks —
// and the force flag. One type for all seven resources because the body is
// one contract; the per-feature copies this replaces were byte-identical.
//
// `force` is optional because the wire omits it on the first pass (the
// backend defaults it to false) and because secrets have no force path at
// all: their single-item delete has no force parameter either, so the
// secrets service never sends the flag and the bulk-remove dialog is told
// `force={false}` for that resource. `bulkRemoveBody` (shared/utils/api.ts)
// is the one place that decides whether the flag travels.
export interface BulkRemoveInput {
	ids: string[];
	force?: boolean;
}
