package models

import (
	"time"
)

// User roles. RoleUser is the default for every user; RoleAdmin is
// granted to the first admin created through the first-run setup
// endpoint and by admin-only user management endpoints.
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
	// AccessTokensRevokedAt is the per-user access-token revocation
	// cutoff (issue 441): a password change or admin password reset
	// stamps now here, and every access JWT of the user minted before
	// the stamp fails validation. nil = never revoked.
	AccessTokensRevokedAt *time.Time `json:"-"`
}

// RevokeAccessTokens stamps the per-user access-token revocation
// cutoff (issue 441): every access JWT of the user minted before the
// stamp fails validation. It is the one owner of the stamping ritual —
// both password paths (the self-service change in the auth domain and
// the admin reset in the users domain) call it in the same user-row
// write that replaces the hash, so the stamp's shape cannot drift
// between them. Second precision, UTC — the jwt NumericDate
// granularity — so a fresh token minted after the call (iat >= cutoff)
// passes the check while every older token fails it.
func (u *User) RevokeAccessTokens() {
	cutoff := time.Now().UTC().Truncate(time.Second)
	u.AccessTokensRevokedAt = &cutoff
}

// ValidRole reports whether role is one of the known role values.
// The users.role column has no DB-level CHECK (SQLite cannot ALTER a
// constraint onto an existing table), so the repository and the user
// service enforce this at the application boundary.
func ValidRole(role string) bool {
	return role == RoleUser || role == RoleAdmin
}
