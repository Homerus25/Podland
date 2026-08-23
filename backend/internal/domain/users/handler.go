package users

import (
	"encoding/json"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
)

// UserHandler handles user management requests.
type UserHandler struct {
	service      *UserService
	errorHandler *errors.ErrorHandler
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(service *UserService) *UserHandler {
	return &UserHandler{
		service:      service,
		errorHandler: errors.NewErrorHandler(),
	}
}

// Create handles POST /api/users: an admin creates a new regular user.
// Registration behind JWTMiddleware + RequireAdmin happens in
// internal/app; the handler itself only shapes request and response.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := contextutils.GetRequestID(r.Context())

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithError(w, errors.ErrInvalidRequest, requestID)
		return
	}

	// Missing/empty fields are rejected by the service with
	// ErrMissingRequiredFields (400) — the same validation every other
	// creation path (cmd/initdb, seeding) goes through.
	user, err := h.service.CreateUser(req.Username, req.Password)
	if err != nil {
		errors.RespondWithError(w, h.errorHandler.HandleError(err), requestID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toUserResponse(user))
}
