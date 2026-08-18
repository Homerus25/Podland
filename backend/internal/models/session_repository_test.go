package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/migrations"
)

// newSession builds a session row with sensible test defaults; callers
// override fields (typically ExpiresAt) before persisting it.
func newSession(id, userID, tokenHash string, expiresAt time.Time) *Session {
	return &Session{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}
}

// TestSessionRepository_CreateGetByTokenHashRoundTrip asserts a session
// row survives a write→read cycle unchanged via the token-hash lookup
// the future refresh endpoint will use.
func TestSessionRepository_CreateGetByTokenHashRoundTrip(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	user := &User{ID: "session-user-1", Username: "sessioner", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(user), "Should create user the session references")

	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	session := newSession("session-1", user.ID, "abc123hash", expiresAt)
	require.NoError(t, repo.Create(session), "Should create session")

	stored, err := repo.GetByTokenHash("abc123hash")
	require.NoError(t, err, "Should look up session by token hash")
	require.NotNil(t, stored, "Created session should be findable by token hash")
	assert.Equal(t, "session-1", stored.ID, "Session ID should round-trip")
	assert.Equal(t, user.ID, stored.UserID, "Session user ID should round-trip")
	assert.Equal(t, "abc123hash", stored.TokenHash, "Token hash should round-trip")
	assert.Equal(t, expiresAt.UTC(), stored.ExpiresAt.UTC(), "Expiry should round-trip")
}

// TestSessionRepository_GetByTokenHashUnknownReturnsNil asserts an
// unknown token hash yields (nil, nil) — the repository's not-found
// convention, matching the user repository — so callers distinguish
// "no session" from a storage failure.
func TestSessionRepository_GetByTokenHashUnknownReturnsNil(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)

	stored, err := repo.GetByTokenHash("no-such-hash")
	require.NoError(t, err, "Unknown token hash is not an error")
	assert.Nil(t, stored, "Unknown token hash should return nil session")
}

// TestSessionRepository_DeleteByTokenHash asserts delete removes exactly
// the row addressed by its token hash and leaves sibling sessions alone.
func TestSessionRepository_DeleteByTokenHash(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	user := &User{ID: "session-user-2", Username: "deleter", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(user), "Should create user")

	future := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, repo.Create(newSession("session-del", user.ID, "hash-to-delete", future)))
	require.NoError(t, repo.Create(newSession("session-keep", user.ID, "hash-to-keep", future)))

	require.NoError(t, repo.DeleteByTokenHash("hash-to-delete"), "Should delete by token hash")

	gone, err := repo.GetByTokenHash("hash-to-delete")
	require.NoError(t, err, "Lookup after delete should not error")
	assert.Nil(t, gone, "Deleted session should no longer be findable")

	kept, err := repo.GetByTokenHash("hash-to-keep")
	require.NoError(t, err, "Sibling lookup should not error")
	assert.NotNil(t, kept, "Sibling session should survive the delete")
}

// TestSessionRepository_DeleteExpiredForUser asserts lazy cleanup
// removes only that user's expired rows: fresh sessions of the same
// user and expired rows belonging to other users must survive.
func TestSessionRepository_DeleteExpiredForUser(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	alice := &User{ID: "session-user-alice", Username: "alice", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	bob := &User{ID: "session-user-bob", Username: "bob", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(alice), "Should create alice")
	require.NoError(t, users.Create(bob), "Should create bob")

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)

	require.NoError(t, repo.Create(newSession("alice-expired-1", alice.ID, "alice-old-1", past)))
	require.NoError(t, repo.Create(newSession("alice-expired-2", alice.ID, "alice-old-2", past)))
	require.NoError(t, repo.Create(newSession("alice-fresh", alice.ID, "alice-new", future)))
	require.NoError(t, repo.Create(newSession("bob-expired", bob.ID, "bob-old", past)))

	require.NoError(t, repo.DeleteExpiredForUser(alice.ID), "Should delete alice's expired sessions")

	stillThere := []string{"alice-new", "bob-old"}
	for _, hash := range stillThere {
		stored, err := repo.GetByTokenHash(hash)
		require.NoError(t, err, "Lookup for %s should not error", hash)
		assert.NotNil(t, stored, "Session %s should survive DeleteExpiredForUser(alice)", hash)
	}
	gone := []string{"alice-old-1", "alice-old-2"}
	for _, hash := range gone {
		stored, err := repo.GetByTokenHash(hash)
		require.NoError(t, err, "Lookup for %s should not error", hash)
		assert.Nil(t, stored, "Expired session %s should be gone", hash)
	}
}

// TestMigrations_SessionsTableIdempotent asserts the migration runner
// applies the sessions migration and that re-running all migrations on
// an already-migrated database is a no-op (the runner records executed
// files, and the SQL itself is IF NOT EXISTS).
func TestMigrations_SessionsTableIdempotent(t *testing.T) {
	db := newTestUserDB(t)

	require.NoError(t, migrations.Run(db), "Second migrations.Run over the sessions migration must succeed")

	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'").Scan(&name)
	require.NoError(t, err, "sessions table should exist after migrations")
	assert.Equal(t, "sessions", name, "sessions table should be present")
}
