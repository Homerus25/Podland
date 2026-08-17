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

// User represents a user in the system
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
}

// validRole reports whether role is one of the known role values.
// The users.role column has no DB-level CHECK (SQLite cannot ALTER a
// constraint onto an existing table), so the repository enforces this
// at the application boundary.
func validRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}
