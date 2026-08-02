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
	ErrMachineNotFound       = NewAppError("MACHINE_NOT_FOUND", "Machine not found", http.StatusNotFound)
	ErrMachineAlreadyExists  = NewAppError("MACHINE_ALREADY_EXISTS", "Machine with this name already exists", http.StatusBadRequest)
	ErrConnectionFailed      = NewAppError("CONNECTION_FAILED", "Failed to connect to machine", http.StatusBadRequest)
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
	ErrUsernameTaken      = NewAppError("USERNAME_TAKEN", "Username already exists", http.StatusBadRequest)
)

// Rate Limiting
var (
	ErrRateLimitExceeded = NewAppError("RATE_LIMIT_EXCEEDED", "Too many requests", http.StatusTooManyRequests)
)

// Containers
var (
	ErrContainerNotFound       = NewAppError("CONTAINER_NOT_FOUND", "Container not found", http.StatusNotFound)
	ErrContainerAlreadyRunning  = NewAppError("CONTAINER_ALREADY_RUNNING", "Container is already running", http.StatusBadRequest)
	ErrContainerNotRunning      = NewAppError("CONTAINER_NOT_RUNNING", "Container is not running", http.StatusBadRequest)
	ErrContainerCreationFailed  = NewAppError("CONTAINER_CREATION_FAILED", "Failed to create container", http.StatusInternalServerError)
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
	ErrQuadletNotFound        = NewAppError("QUADLET_NOT_FOUND", "Quadlet not found", http.StatusNotFound)
	ErrQuadletAlreadyExists   = NewAppError("QUADLET_ALREADY_EXISTS", "Quadlet with this name already exists", http.StatusConflict)
	ErrQuadletResourceRunning = NewAppError("QUADLET_RESOURCE_RUNNING", "Quadlet-managed resource is running", http.StatusBadRequest)
	ErrQuadletAlreadyRunning   = NewAppError("QUADLET_ALREADY_RUNNING", "Quadlet is already running", http.StatusBadRequest)
	ErrQuadletNotRunning       = NewAppError("QUADLET_NOT_RUNNING", "Quadlet is not running", http.StatusBadRequest)
	ErrQuadletGenerationFailed = NewAppError("QUADLET_GENERATION_FAILED", "Failed to generate quadlet", http.StatusInternalServerError)
)

// Access
var (
	ErrAccessDenied        = NewAppError("ACCESS_DENIED", "User does not have permission to access this resource", http.StatusForbidden)
	ErrAccessAlreadyGranted = NewAppError("ACCESS_ALREADY_GRANTED", "Access already granted to this user", http.StatusBadRequest)
	ErrAccessNotFound      = NewAppError("ACCESS_NOT_FOUND", "Access entry not found", http.StatusNotFound)
	ErrCannotRevokeOwner   = NewAppError("CANNOT_REVOKE_OWNER", "Cannot revoke access from machine owner", http.StatusBadRequest)
)

// Generic
var (
	ErrInternalError      = NewAppError("INTERNAL_ERROR", "An unexpected error occurred", http.StatusInternalServerError)
	ErrServiceUnavailable = NewAppError("SERVICE_UNAVAILABLE", "Service temporarily unavailable", http.StatusServiceUnavailable)
	ErrInvalidRequest      = NewAppError("INVALID_REQUEST", "Invalid request", http.StatusBadRequest)
	ErrNotImplemented      = NewAppError("NOT_IMPLEMENTED", "Feature not implemented", http.StatusNotImplemented)
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
