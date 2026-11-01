package models

import (
	"database/sql"
	"fmt"
	"time"
)

// RevokedTokenRepository handles database operations for the access-token
// denylist (the revoked_access_tokens table, issue 441): one row per
// revoked access JWT, keyed by the token's jti and carrying the token's
// own exp — the row's purge deadline.
type RevokedTokenRepository struct {
	db *sql.DB
}

// RevokedTokenRepositoryInterface defines the interface for denylist
// operations. IsRevoked answers false for an unknown jti (never minted,
// already purged) — the repository-wide not-found convention — so the
// auth service's ValidateToken check distinguishes "not revoked" from a
// storage failure.
type RevokedTokenRepositoryInterface interface {
	// Create denylists a jti until the given expiry (the revoked
	// token's own exp). It is idempotent: re-denying an already
	// denylisted jti is a silent success, so a replayed or concurrent
	// logout never surfaces a constraint error.
	Create(jti string, expiresAt time.Time) error
	IsRevoked(jti string) (bool, error)
	// DeleteExpired lazily purges rows whose expiry has passed (login
	// triggers it; there is no background sweeper) — a denylist row is
	// dead weight once the token it addresses would fail validation
	// anyway.
	DeleteExpired() error
}

// NewRevokedTokenRepository creates a new RevokedTokenRepository
func NewRevokedTokenRepository(db *sql.DB) *RevokedTokenRepository {
	return &RevokedTokenRepository{db: db}
}

// Create inserts a denylist row. INSERT OR IGNORE keeps logout
// idempotent: the jti is the primary key, and a replayed logout (or two
// concurrent ones) re-inserting the same row is the same end state.
func (r *RevokedTokenRepository) Create(jti string, expiresAt time.Time) error {
	query := `INSERT OR IGNORE INTO revoked_access_tokens (jti, expires_at) VALUES (?, ?)`

	_, err := r.db.Exec(query, jti, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to revoke access token: %w", err)
	}

	return nil
}

// IsRevoked reports whether the jti is denylisted
func (r *RevokedTokenRepository) IsRevoked(jti string) (bool, error) {
	query := `SELECT COUNT(*) FROM revoked_access_tokens WHERE jti = ?`

	var count int
	err := r.db.QueryRow(query, jti).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check revoked access token: %w", err)
	}

	return count > 0, nil
}

// DeleteExpired removes every denylist row whose expiry has passed
func (r *RevokedTokenRepository) DeleteExpired() error {
	query := `DELETE FROM revoked_access_tokens WHERE expires_at <= ?`

	_, err := r.db.Exec(query, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to delete expired revoked access tokens: %w", err)
	}

	return nil
}
