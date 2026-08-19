package models

import (
	"database/sql"
	"fmt"
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

// TestSessionRepository_RotateReplacesSession asserts rotation is a
// swap: the row addressed by the old token hash disappears, the new
// session row takes its place, and sibling sessions are untouched.
func TestSessionRepository_RotateReplacesSession(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	user := &User{ID: "session-user-rotate", Username: "rotator", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(user), "Should create user")

	future := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, repo.Create(newSession("session-old", user.ID, "old-hash", future)))
	require.NoError(t, repo.Create(newSession("session-sibling", user.ID, "sibling-hash", future)))

	replacement := newSession("session-new", user.ID, "new-hash", time.Now().UTC().Add(48*time.Hour).Truncate(time.Second))
	rotated, err := repo.Rotate("old-hash", replacement)
	require.NoError(t, err, "Rotate should not error")
	assert.True(t, rotated, "Existing session should rotate")

	gone, err := repo.GetByTokenHash("old-hash")
	require.NoError(t, err, "Lookup of rotated-out hash should not error")
	assert.Nil(t, gone, "Old hash must no longer address a session")

	inserted, err := repo.GetByTokenHash("new-hash")
	require.NoError(t, err, "Lookup of rotated-in hash should not error")
	require.NotNil(t, inserted, "New hash should address the rotated session")
	assert.Equal(t, "session-new", inserted.ID, "Rotated session ID should round-trip")
	assert.Equal(t, replacement.ExpiresAt.UTC(), inserted.ExpiresAt.UTC(), "Rotated expiry should round-trip")

	sibling, err := repo.GetByTokenHash("sibling-hash")
	require.NoError(t, err, "Sibling lookup should not error")
	assert.NotNil(t, sibling, "Sibling session must survive another session's rotation")
}

// TestSessionRepository_RotateUnknownHashIsNoOp asserts a rotation whose
// old hash addresses nothing reports false, is not an error, and leaves
// no new row behind (the replay path must not resurrect sessions).
func TestSessionRepository_RotateUnknownHashIsNoOp(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	user := &User{ID: "session-user-norot", Username: "norot", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(user), "Should create user")

	replacement := newSession("session-ghost", user.ID, "ghost-hash", time.Now().UTC().Add(24*time.Hour))
	rotated, err := repo.Rotate("no-such-hash", replacement)
	require.NoError(t, err, "Unknown hash is not an error")
	assert.False(t, rotated, "Unknown hash should not rotate")

	ghost, err := repo.GetByTokenHash("ghost-hash")
	require.NoError(t, err, "Lookup of replacement hash should not error")
	assert.Nil(t, ghost, "Failed rotation must not leave the new row behind")
}

// TestSessionRepository_RotateConcurrentExactlyOneWins asserts the
// atomicity the refresh endpoint relies on: two concurrent rotations of
// the same token hash cannot both succeed. Exactly one goroutine swaps
// the row; the loser reports false, and the table is left with exactly
// one live row (the winner's) — never zero, never two.
func TestSessionRepository_RotateConcurrentExactlyOneWins(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewSessionRepository(db)
	users := NewUserRepository(db)

	user := &User{ID: "session-user-race", Username: "racer", PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	require.NoError(t, users.Create(user), "Should create user")

	require.NoError(t, repo.Create(newSession("session-raced", user.ID, "raced-hash", time.Now().UTC().Add(24*time.Hour))))

	winners := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			replacement := newSession(
				fmt.Sprintf("session-winner-%d", i),
				user.ID,
				fmt.Sprintf("winner-hash-%d", i),
				time.Now().UTC().Add(48*time.Hour),
			)
			rotated, err := repo.Rotate("raced-hash", replacement)
			if err != nil {
				t.Errorf("concurrent Rotate failed: %v", err)
				winners <- false
				return
			}
			winners <- rotated
		}(i)
	}

	first := <-winners
	second := <-winners
	assert.NotEqual(t, first, second,
		"Exactly one concurrent rotation must win (got %t and %t)", first, second)

	sessions := sessionRowsForUser(t, db, user.ID)
	assert.Len(t, sessions, 1, "The race must leave exactly one live session row")
	assert.NotEqual(t, "raced-hash", sessions[0],
		"The surviving row must be a winner's replacement, not the raced original")
}

// sessionRowsForUser reads the given user's token hashes straight from
// the sessions table, ordered by creation, so rotation tests can assert
// on committed state rather than going through the repository again.
func sessionRowsForUser(t *testing.T, db *sql.DB, userID string) []string {
	t.Helper()

	rows, err := db.Query(`SELECT token_hash FROM sessions WHERE user_id = ? ORDER BY created_at, id`, userID)
	require.NoError(t, err, "Should query sessions for user")
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		require.NoError(t, rows.Scan(&hash), "Should scan token hash")
		hashes = append(hashes, hash)
	}
	require.NoError(t, rows.Err(), "Should iterate session rows")
	return hashes
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
