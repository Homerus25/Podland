package models

import (
	"database/sql"
	"fmt"
	"time"
)

// SessionRepository handles database operations for refresh sessions
type SessionRepository struct {
	db *sql.DB
}

// SessionRepositoryInterface defines the interface for session repository
// operations. GetByTokenHash follows the repository-wide not-found
// convention: (nil, nil) when no row matches, so callers distinguish
// "no session" from a storage failure.
type SessionRepositoryInterface interface {
	Create(session *Session) error
	GetByTokenHash(tokenHash string) (*Session, error)
	DeleteByTokenHash(tokenHash string) error
	DeleteExpiredForUser(userID string) error
	// Rotate atomically swaps the session addressed by oldTokenHash for
	// newSession: one transaction deletes the old row and inserts the
	// new one. It reports whether the swap happened — false (with a nil
	// error) means the old row was already gone, so the caller must
	// treat the presented token as invalid (replayed after rotation,
	// revoked, or beaten by a concurrent refresh).
	Rotate(oldTokenHash string, newSession *Session) (bool, error)
}

// NewSessionRepository creates a new SessionRepository
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session row
func (r *SessionRepository) Create(session *Session) error {
	query := `INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, session.ID, session.UserID, session.TokenHash, session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetByTokenHash retrieves a session by its token hash
func (r *SessionRepository) GetByTokenHash(tokenHash string) (*Session, error) {
	query := `SELECT id, user_id, token_hash, created_at, expires_at FROM sessions WHERE token_hash = ?`

	var session Session
	err := r.db.QueryRow(query, tokenHash).Scan(&session.ID, &session.UserID, &session.TokenHash, &session.CreatedAt, &session.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// DeleteByTokenHash removes the session addressed by its token hash
func (r *SessionRepository) DeleteByTokenHash(tokenHash string) error {
	query := `DELETE FROM sessions WHERE token_hash = ?`

	_, err := r.db.Exec(query, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// Rotate atomically replaces the session addressed by oldTokenHash with
// newSession. The delete and the insert share one transaction, so two
// concurrent rotations of the same token cannot both succeed: the loser's
// delete affects zero rows, the transaction aborts, and Rotate reports
// false.
func (r *SessionRepository) Rotate(oldTokenHash string, newSession *Session) (bool, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin session rotation: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`DELETE FROM sessions WHERE token_hash = ?`, oldTokenHash)
	if err != nil {
		return false, fmt.Errorf("failed to delete rotated-out session: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to count deleted sessions: %w", err)
	}
	if deleted == 0 {
		return false, nil
	}

	_, err = tx.Exec(`INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		newSession.ID, newSession.UserID, newSession.TokenHash, newSession.CreatedAt, newSession.ExpiresAt)
	if err != nil {
		return false, fmt.Errorf("failed to insert rotated session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit session rotation: %w", err)
	}
	return true, nil
}

// DeleteExpiredForUser lazily purges the given user's expired session
// rows (login triggers it; there is no background sweeper)
func (r *SessionRepository) DeleteExpiredForUser(userID string) error {
	query := `DELETE FROM sessions WHERE user_id = ? AND expires_at <= ?`

	_, err := r.db.Exec(query, userID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return nil
}
