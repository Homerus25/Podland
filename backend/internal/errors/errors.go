package errors

import (
	"fmt"
	"net/http"
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
	ErrTokenExpired       = NewAppError("TOKEN_EXPIRED", "Token has expired", http.StatusUnauthorized)
	ErrTokenInvalid       = NewAppError("TOKEN_INVALID", "Invalid token", http.StatusUnauthorized)
	ErrUserNotFound       = NewAppError("USER_NOT_FOUND", "User not found", http.StatusNotFound)
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
)

// Secrets
var (
	ErrSecretNotFound       = NewAppError("SECRET_NOT_FOUND", "Secret not found", http.StatusNotFound)
	ErrSecretAlreadyExists  = NewAppError("SECRET_ALREADY_EXISTS", "Secret with this name already exists", http.StatusConflict)
	ErrSecretInUse          = NewAppError("SECRET_IN_USE", "Secret is in use by one or more containers", http.StatusConflict)
	ErrSecretCreationFailed = NewAppError("SECRET_CREATION_FAILED", "Failed to create secret", http.StatusInternalServerError)
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
	ErrNotImplemented     = NewAppError("NOT_IMPLEMENTED", "Feature not implemented", http.StatusNotImplemented)
)

// ErrorHandler handles error responses
type ErrorHandler struct{}

// NewErrorHandler creates a new ErrorHandler
func NewErrorHandler() *ErrorHandler {
	return &ErrorHandler{}
}

// HandleError converts an error to an AppError
func (h *ErrorHandler) HandleError(err error) *AppError {
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return ErrInternalError
}
