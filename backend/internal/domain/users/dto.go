package users

import (
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
)

// CreateUserRequest is the body of POST /api/users. Role is optional:
// an omitted or empty role creates a regular user, "admin" creates an
// administrator — any other value is rejected by the service.
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UpdateRoleRequest is the body of PUT /api/users/{id}/role. The role
// must be "user" or "admin" — the tags reject anything else (an empty
// one included) at bind time with the envelope's standard
// VALIDATION_FAILED; the service re-checks the same rule as a backstop,
// keeping the role vocabulary single-owned there like on the create
// path.
type UpdateRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=user admin"`
}

// ResetPasswordRequest is the body of PUT /api/users/{id}/password.
// The admin supplies only the new password — there is no
// current-password check, because the caller is not the target; the
// length policy lives in the service, like on every other password
// path.
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" validate:"required"`
}

// UserResponse is the wire representation of a user. It intentionally
// mirrors the public subset of models.User — the password hash is never
// serialized here.
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// UserSummaryResponse is the minimal wire representation of a user:
// id + username, enough to pick a person when sharing a machine. Every
// authenticated caller receives it; only admins receive the full
// UserResponse on top (see UserHandler.List).
type UserSummaryResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// toUserResponse maps a persisted user to its wire representation.
func toUserResponse(user *models.User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
}

// toUserSummaryResponse maps a persisted user to its minimal wire
// representation.
func toUserSummaryResponse(user *models.User) UserSummaryResponse {
	return UserSummaryResponse{
		ID:       user.ID,
		Username: user.Username,
	}
}

// toUserResponses maps users to their full wire representations — the
// admin list shape.
func toUserResponses(users []*models.User) []UserResponse {
	responses := make([]UserResponse, len(users))
	for i, user := range users {
		responses[i] = toUserResponse(user)
	}
	return responses
}

// toUserSummaryResponses maps users to their minimal wire
// representations — the every-authenticated-caller list shape.
func toUserSummaryResponses(users []*models.User) []UserSummaryResponse {
	summaries := make([]UserSummaryResponse, len(users))
	for i, user := range users {
		summaries[i] = toUserSummaryResponse(user)
	}
	return summaries
}
