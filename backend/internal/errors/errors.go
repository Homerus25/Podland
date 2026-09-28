package errors

import (
	"fmt"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/models"
)

// AppError represents an application error
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithMessage returns a copy of the error carrying the given message.
// The sentinels are shared pointers, so callers must never mutate them in
// place; a copy keeps the sentinel (and its code/status) intact while the
// message carries per-request detail. Only for non-5xx codes:
// RespondWithError rewrites 5xx messages as generic.
func (e *AppError) WithMessage(message string) *AppError {
	copied := *e
	copied.Message = message
	return &copied
}

// NewAppError creates a new AppError
func NewAppError(code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Status:  status,
	}
}

// Error codes for different domains

// Machines
var (
	ErrMachineNotFound      = NewAppError("MACHINE_NOT_FOUND", "Machine not found", http.StatusNotFound)
	ErrMachineAlreadyExists = NewAppError("MACHINE_ALREADY_EXISTS", "Machine with this name already exists", http.StatusBadRequest)
	// ErrConnectionFailed is the RUNTIME connection failure: a persisted
	// machine's remote podman socket could not be reached during an
	// operation (containers/images/quadlets). It is a gateway error (502)
	// because the backend acts as a gateway to the remote podman.
	ErrConnectionFailed = NewAppError("CONNECTION_FAILED", "Failed to connect to machine", http.StatusBadGateway)
	// ErrConnectionTestFailed is the REGISTRATION connection failure: the
	// credentials/target a client just submitted did not produce a working
	// SSH+podman connection during the synchronous registration probe
	// (description_backend.md §Machine Registration step 5). This is a
	// client error (400) — the user supplied a bad host, bad credentials,
	// or a wrong socket path — so it carries the same CONNECTION_FAILED
	// code as the runtime case but the 4xx status the documented
	// registration flow requires. Two sentinels share a code string
	// deliberately: RespondWithError drives the HTTP status off each
	// instance's Status, so the registration (400) and runtime (502)
	// paths surface correctly without a code→status lookup. They remain
	// DISTINCT pointers (AppError has no Is/Unwrap), so errors.Is and
	// pointer equality distinguish a registration failure from a runtime
	// failure — do not collapse them.
	ErrConnectionTestFailed  = NewAppError("CONNECTION_FAILED", "Failed to connect to machine during registration", http.StatusBadRequest)
	ErrConnectionTestTimeout = NewAppError("CONNECTION_TEST_TIMEOUT", "Connection test timed out", http.StatusBadRequest)
	ErrInvalidSSHKey         = NewAppError("INVALID_SSH_KEY", "Invalid SSH private key format", http.StatusBadRequest)
	ErrMissingRequiredFields = NewAppError("VALIDATION_FAILED", "Missing required fields", http.StatusBadRequest)
)

// Auth
var (
	ErrInvalidCredentials = NewAppError("INVALID_CREDENTIALS", "Invalid username or password", http.StatusUnauthorized)
	// ErrTokenMissing is the 401 for a request that presented no
	// Authorization header at all. Distinct from ErrTokenInvalid: a
	// malformed header or bad token means credentials were presented
	// and rejected; a missing header means there were none. See
	// middleware.bearerClaims for the branch.
	ErrTokenMissing = NewAppError("TOKEN_MISSING", "No token provided", http.StatusUnauthorized)
	ErrTokenExpired = NewAppError("TOKEN_EXPIRED", "Token has expired", http.StatusUnauthorized)
	ErrTokenInvalid = NewAppError("TOKEN_INVALID", "Invalid token", http.StatusUnauthorized)
	ErrUserNotFound = NewAppError("USER_NOT_FOUND", "User not found", http.StatusNotFound)
	// ErrAdminRequired backs the admin-only routes (starting with
	// POST /api/users): the caller is authenticated but lacks the
	// admin role.
	ErrAdminRequired = NewAppError("ADMIN_REQUIRED", "Admin privileges required", http.StatusForbidden)
	// ErrUsernameTaken is 409 Conflict: the username is already taken.
	// The service's exists-check returns it directly; the users.username
	// UNIQUE constraint can also surface it as a backstop when two
	// concurrent creations race past the check.
	ErrUsernameTaken = NewAppError("USERNAME_TAKEN", "Username already exists", http.StatusConflict)
	// ErrRefreshInvalid is the refresh path's 401 for a missing cookie
	// or an unknown token hash. Replayed (already-rotated) and revoked
	// tokens land here too: their rows are gone, so the lookup misses
	// and the failure is indistinguishable from never-having-had one.
	ErrRefreshInvalid = NewAppError("AUTH_REFRESH_INVALID", "Invalid refresh token", http.StatusUnauthorized)
	// ErrSessionExpired is the refresh path's 401 for a session row that
	// exists but is past its expiry — the row is deleted on encounter as
	// lazy cleanup, so the next attempt reports ErrRefreshInvalid.
	ErrSessionExpired = NewAppError("AUTH_SESSION_EXPIRED", "Session expired", http.StatusUnauthorized)
	// ErrPasswordTooShort is the password policy's 400: a password
	// below models.MinPasswordLength. Introduced by the self-service
	// password change; the series' admin create/reset paths reuse it so
	// the wire contract — and the UI's field-level mapping — stays
	// uniform across every password-setting path. The message derives
	// from the constant, so bumping the policy floor updates the wire
	// text instead of leaving it lying.
	ErrPasswordTooShort = NewAppError("PASSWORD_TOO_SHORT",
		fmt.Sprintf("Password must be at least %d characters", models.MinPasswordLength),
		http.StatusBadRequest)
	// ErrInvalidRole is the user-creation path's 400 for a role outside
	// {user, admin}. Shares VALIDATION_FAILED with
	// ErrMissingRequiredFields so the wire contract stays uniform; the
	// message names the offending field.
	ErrInvalidRole = NewAppError("VALIDATION_FAILED", "Role must be \"user\" or \"admin\"", http.StatusBadRequest)
	// ErrSelfRoleChange is the role-change endpoint's self-lockout
	// guard: an admin may not change their own role, so the instance
	// can never lose its administration by accident. The bootstrap CLI
	// remains the escape hatch for a genuinely intended self-demotion.
	ErrSelfRoleChange = NewAppError("SELF_ROLE_CHANGE", "Cannot change your own role", http.StatusBadRequest)
	// ErrSelfPasswordReset is the admin password-reset endpoint's
	// self-lockout guard: an admin may not reset their own password
	// through the admin flow — self-service change password is the one
	// self path (and the only one that returns a fresh session). The
	// guard runs before the acting-admin password verification, so a
	// doomed request burns neither a bcrypt compare nor failure budget.
	ErrSelfPasswordReset = NewAppError("SELF_PASSWORD_RESET", "Cannot reset your own password", http.StatusBadRequest)
	// ErrSetupCompleted is the first-run setup endpoint's 409: the
	// instance already has a user, so the one-time setup offer is gone.
	// It answers both a losing concurrent setup request (the winner's
	// transaction committed first) and any later call on a configured
	// instance — in both cases nothing was written.
	ErrSetupCompleted = NewAppError("SETUP_COMPLETED", "Setup has already been completed", http.StatusConflict)
)

// Rate Limiting
var (
	ErrRateLimitExceeded = NewAppError("RATE_LIMIT_EXCEEDED", "Too many requests", http.StatusTooManyRequests)
)

// Containers
var (
	ErrContainerNotFound       = NewAppError("CONTAINER_NOT_FOUND", "Container not found", http.StatusNotFound)
	ErrContainerAlreadyRunning = NewAppError("CONTAINER_ALREADY_RUNNING", "Container is already running", http.StatusBadRequest)
	ErrContainerNotRunning     = NewAppError("CONTAINER_NOT_RUNNING", "Container is not running", http.StatusBadRequest)
	ErrContainerCreationFailed = NewAppError("CONTAINER_CREATION_FAILED", "Failed to create container", http.StatusInternalServerError)
	ErrPodNotFound             = NewAppError("POD_NOT_FOUND", "Pod not found", http.StatusNotFound)
)

// Images
var (
	ErrImageNotFound         = NewAppError("IMAGE_NOT_FOUND", "Image not found", http.StatusNotFound)
	ErrImagePullFailed       = NewAppError("IMAGE_PULL_FAILED", "Failed to pull image", http.StatusInternalServerError)
	ErrImageUsedByContainers = NewAppError("IMAGE_USED_BY_CONTAINERS", "Image is used by one or more containers", http.StatusBadRequest)
	ErrOperationCancelled    = NewAppError("OPERATION_CANCELLED", "Operation was cancelled by the client", http.StatusGatewayTimeout)
)

// Quadlets
var (
	ErrQuadletNotFound         = NewAppError("QUADLET_NOT_FOUND", "Quadlet not found", http.StatusNotFound)
	ErrQuadletAlreadyExists    = NewAppError("QUADLET_ALREADY_EXISTS", "Quadlet with this name already exists", http.StatusConflict)
	ErrQuadletResourceRunning  = NewAppError("QUADLET_RESOURCE_RUNNING", "Quadlet-managed resource is running", http.StatusBadRequest)
	ErrQuadletAlreadyRunning   = NewAppError("QUADLET_ALREADY_RUNNING", "Quadlet is already running", http.StatusBadRequest)
	ErrQuadletNotRunning       = NewAppError("QUADLET_NOT_RUNNING", "Quadlet is not running", http.StatusBadRequest)
	ErrQuadletGenerationFailed = NewAppError("QUADLET_GENERATION_FAILED", "Failed to generate quadlet", http.StatusInternalServerError)
	// ErrQuadletStopTimedOut is the stop/restart wait give-up answer: the
	// coreutils timeout wrapper around `systemctl stop/restart` expired
	// (exit 124) before the unit's stop job finished. 504 Gateway Timeout —
	// the backend acts as a gateway to the remote systemd and gave up
	// waiting. It says the WAIT was given up, not that the stop failed:
	// the stop job may still complete in systemd after expiry.
	ErrQuadletStopTimedOut = NewAppError("QUADLET_STOP_TIMED_OUT", "Timed out waiting for the quadlet to stop", http.StatusGatewayTimeout)
)

// Secrets
var (
	ErrSecretNotFound       = NewAppError("SECRET_NOT_FOUND", "Secret not found", http.StatusNotFound)
	ErrSecretAlreadyExists  = NewAppError("SECRET_ALREADY_EXISTS", "Secret with this name already exists", http.StatusConflict)
	ErrSecretInUse          = NewAppError("SECRET_IN_USE", "Secret is in use by one or more containers", http.StatusConflict)
	ErrSecretCreationFailed = NewAppError("SECRET_CREATION_FAILED", "Failed to create secret", http.StatusInternalServerError)
)

// Volumes
var (
	ErrVolumeNotFound = NewAppError("VOLUME_NOT_FOUND", "Volume not found", http.StatusNotFound)
	// ErrVolumeInUse is 409: podman refuses to remove a volume that is
	// in use by a container unless force is requested. Distinct code so
	// the UI can explain what depends on the volume.
	ErrVolumeInUse = NewAppError("VOLUME_IN_USE", "Volume is in use by a container", http.StatusConflict)
	// ErrVolumeUploadTooLarge is 413: a populate upload exceeded
	// VOLUME_UPLOAD_MAX_BYTES while streaming to disk.
	ErrVolumeUploadTooLarge = NewAppError("VOLUME_UPLOAD_TOO_LARGE", "Upload exceeds the maximum allowed size", http.StatusRequestEntityTooLarge)
)

// Access
var (
	ErrAccessDenied         = NewAppError("ACCESS_DENIED", "User does not have permission to access this resource", http.StatusForbidden)
	ErrAccessAlreadyGranted = NewAppError("ACCESS_ALREADY_GRANTED", "Access already granted to this user", http.StatusBadRequest)
	ErrAccessNotFound       = NewAppError("ACCESS_NOT_FOUND", "Access entry not found", http.StatusNotFound)
	ErrCannotRevokeOwner    = NewAppError("CANNOT_REVOKE_OWNER", "Cannot revoke access from machine owner", http.StatusBadRequest)
)

// Generic
var (
	ErrInternalError      = NewAppError("INTERNAL_ERROR", "An unexpected error occurred", http.StatusInternalServerError)
	ErrServiceUnavailable = NewAppError("SERVICE_UNAVAILABLE", "Service temporarily unavailable", http.StatusServiceUnavailable)
	ErrInvalidRequest     = NewAppError("INVALID_REQUEST", "Invalid request", http.StatusBadRequest)
	// ErrValidationFailed backs web.Decode's struct-validation branch: the
	// request body decoded as JSON but failed its validate tags. Minted
	// only by Decode (WithMessage carries the validator's message).
	ErrValidationFailed = NewAppError("VALIDATION_FAILED", "Validation failed", http.StatusBadRequest)
	ErrNotImplemented   = NewAppError("NOT_IMPLEMENTED", "Feature not implemented", http.StatusNotImplemented)
)
