package models

import (
	"time"
)

// User roles. RoleUser is the default for every user; RoleAdmin is
// granted to the bootstrap user created by cmd/initdb and (later) by
// admin-only user management endpoints.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// MinPasswordLength is the password policy's minimum. The self-service
// password change enforces it; the series' admin create/reset paths
// enforce the same floor.
const MinPasswordLength = 8

// User represents a user in the system
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ValidRole reports whether role is one of the known role values.
// The users.role column has no DB-level CHECK (SQLite cannot ALTER a
// constraint onto an existing table), so the repository and the user
// service enforce this at the application boundary.
func ValidRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}
