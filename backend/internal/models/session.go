package models

import (
	"time"
)

// Session is one refresh session: a row created at login whose
// token_hash is the SHA-256 hex digest of an opaque refresh token.
// The plaintext token is never persisted — it lives only in the
// httpOnly cookie handed to the client.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	TokenHash string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}
