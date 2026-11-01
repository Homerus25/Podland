package users

import (
	"context"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/web"
)

// UserHandler handles user management requests.
type UserHandler struct {
	service     *UserService
	resetBudget *failurebudget.Surface
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(service *UserService) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

// WithFailureBudget wires the reset endpoint's credential-failure
// budget, so the handler can advertise its X-RateLimit-* trio on the
// 429 answers the service's budget rejections produce. The fluent
// shape mirrors the auth handler's budget setters: an optional
// dependency layered onto the one-argument constructor. Unwired,
// budget 429s (unreachable when the service's brake is unwired too)
// carry no rate-limit headers.
func (h *UserHandler) WithFailureBudget(budget *failurebudget.Surface) *UserHandler {
	h.resetBudget = budget
	return h
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
		if appErr := web.Decode(w, r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		// Missing/empty required fields and an unknown role are rejected
		// by the service (400) — the same validation every other creation
		// path (setup endpoint, seeding) goes through.
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
		if appErr := web.Decode(w, r, &req); appErr != nil {
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
// sets a new password for another user, re-authenticating with their
// own password (the body's adminPassword — verified against the acting
// admin by the service, which also spends the caller-keyed failure
// budget on misses and refuses a self reset) and revoking every session
// of the target (the length policy and the revocation live in the
// service; no fresh login is issued — the target must log in with the
// new password). On the budget's 429 the reset surface advertises the
// X-RateLimit trio (failurebudget.Surface.Advertise owns the
// classification). Registration behind JWTMiddleware + RequireAdmin
// happens in internal/app; the handler reads the URL param, decodes the
// body, and crosses the envelope.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, callerID string) (UserResponse, error) {
		targetID, guard := web.URLParam(r, "userId", "MISSING_USER_ID")
		if guard != nil {
			return UserResponse{}, guard
		}

		var req ResetPasswordRequest
		if appErr := web.Decode(w, r, &req); appErr != nil {
			return UserResponse{}, appErr
		}

		user, err := h.service.ResetPassword(callerID, targetID, req.AdminPassword, req.NewPassword)
		if err != nil {
			h.resetBudget.Advertise(w, callerID, err)
			return UserResponse{}, err
		}
		return toUserResponse(user), nil
	})
}
