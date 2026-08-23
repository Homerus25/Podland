package users

import (
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
)

// CreateUserRequest is the body of POST /api/users. There is
// deliberately no role field: admins create regular users only, so
// privilege escalation requires direct database access.
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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

// toUserResponse maps a persisted user to its wire representation.
func toUserResponse(user *models.User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
}
