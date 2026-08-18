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
