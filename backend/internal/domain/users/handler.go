package users

import (
	"context"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/web"
)

// UserHandler handles user management requests.
type UserHandler struct {
	service *UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(service *UserService) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

// Create handles POST /api/users: an admin creates a new regular user.
// Registration behind JWTMiddleware + RequireAdmin happens in
// internal/app; the handler decodes the body and crosses the envelope.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusCreated, func(ctx context.Context, userID string) (UserResponse, error) {
		var req CreateUserRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		// Missing/empty fields are rejected by the service with
		// ErrMissingRequiredFields (400) — the same validation every other
		// creation path (cmd/initdb, seeding) goes through.
		user, err := h.service.CreateUser(req.Username, req.Password)
		if err != nil {
			return UserResponse{}, err
		}
		return toUserResponse(user), nil
	})
}
