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

// List handles GET /api/users: any authenticated caller may list users,
// and the response shape depends on the caller's role — admins receive
// the full UserResponse, every other authenticated caller the minimal
// UserSummaryResponse (enough to pick a person when sharing a machine);
// the service resolves the caller's role. Registration behind
// JWTMiddleware happens in internal/app; the handler crosses the
// envelope.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (any, error) {
		users, callerIsAdmin, err := h.service.List(userID)
		if err != nil {
			return nil, err
		}
		if !callerIsAdmin {
			return toUserSummaryResponses(users), nil
		}
		return toUserResponses(users), nil
	})
}

// Create handles POST /api/users: an admin creates a new user, with an
// optional role (omitted means the regular "user" role). Registration
// behind JWTMiddleware + RequireAdmin happens in internal/app; the
// handler decodes the body and crosses the envelope.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusCreated, func(ctx context.Context, userID string) (UserResponse, error) {
		var req CreateUserRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		// Missing/empty required fields and an unknown role are rejected
		// by the service (400) — the same validation every other creation
		// path (cmd/initdb, seeding) goes through.
		user, err := h.service.CreateUserWithRole(req.Username, req.Password, req.Role)
		if err != nil {
			return UserResponse{}, err
		}
		return toUserResponse(user), nil
	})
}

// UpdateRole handles PUT /api/users/{userId}/role: an admin sets
// another user's role. The self-lockout guard (an admin may not change
// their own role) and the role vocabulary live in the service.
// Registration behind JWTMiddleware + RequireAdmin happens in
// internal/app; the handler reads the URL param, decodes the body, and
// crosses the envelope.
func (h *UserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, callerID string) (UserResponse, error) {
		targetID, guard := web.URLParam(r, "userId", "MISSING_USER_ID")
		if guard != nil {
			return UserResponse{}, guard
		}

		var req UpdateRoleRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		user, err := h.service.SetRole(callerID, targetID, req.Role)
		if err != nil {
			return UserResponse{}, err
		}
		return toUserResponse(user), nil
	})
}

// ResetPassword handles PUT /api/users/{userId}/password: an admin
// sets a new password for another user, revoking every session of the
// target (the length policy and the revocation live in the service;
// no fresh login is issued — the target must log in with the new
// password). Registration behind JWTMiddleware + RequireAdmin happens
// in internal/app; the handler reads the URL param, decodes the body,
// and crosses the envelope.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, callerID string) (UserResponse, error) {
		targetID, guard := web.URLParam(r, "userId", "MISSING_USER_ID")
		if guard != nil {
			return UserResponse{}, guard
		}

		var req ResetPasswordRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		user, err := h.service.ResetPassword(targetID, req.NewPassword)
		if err != nil {
			return UserResponse{}, err
		}
		return toUserResponse(user), nil
	})
}
