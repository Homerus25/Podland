package models

import (
	"time"
)

// Session is one refresh session: a row created at login whose
// token_hash is the SHA-256 hex digest of an opaque refresh token.
// The plaintext token is never persisted — it lives only in the
// httpOnly cookie handed to the client. Remembered marks a login that
// opted in via the request's rememberDevice field (issue 379): the
// session lives on the longer remembered TTL, and refresh rotation
// copies the flag onto the replacement row.
type Session struct {
	ID         string    `json:"id"`
	UserID     string    `json:"userId"`
	TokenHash  string    `json:"-"`
	Remembered bool      `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
