package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRevokedTokenRepository_CreateIsRevokedRoundTrip asserts a denylist
// row survives a write→read cycle: a jti inserted with its expiry answers
// IsRevoked true, and an unknown jti answers false (not an error — the
// not-found convention the ValidateToken check consumes).
func TestRevokedTokenRepository_CreateIsRevokedRoundTrip(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewRevokedTokenRepository(db)

	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	require.NoError(t, repo.Create("jti-1", expiresAt), "Should denylist a jti")

	revoked, err := repo.IsRevoked("jti-1")
	require.NoError(t, err, "Denylist lookup should not error")
	assert.True(t, revoked, "A denylisted jti must answer revoked")

	unknown, err := repo.IsRevoked("never-minted")
	require.NoError(t, err, "Unknown-jti lookup should not error")
	assert.False(t, unknown, "An unknown jti must answer not revoked")
}

// TestRevokedTokenRepository_CreateIsIdempotent asserts denylisting the
// same jti twice is a silent success: logout is idempotent, so a replayed
// logout (or two concurrent ones) must not surface a constraint error.
func TestRevokedTokenRepository_CreateIsIdempotent(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewRevokedTokenRepository(db)

	expiresAt := time.Now().UTC().Add(time.Hour)
	require.NoError(t, repo.Create("jti-same", expiresAt), "First denylist should succeed")

	assert.NoError(t, repo.Create("jti-same", expiresAt), "Re-denying the same jti must not error")

	revoked, err := repo.IsRevoked("jti-same")
	require.NoError(t, err, "Lookup after the re-insert should not error")
	assert.True(t, revoked, "The jti must stay denylisted")
}

// TestRevokedTokenRepository_DeleteExpiredPurgesOnlyExpired asserts the
// lazy purge login triggers removes exactly the rows whose expiry has
// passed — a denylist row is dead weight once the token it addresses
// would fail validation anyway — and leaves live rows untouched.
func TestRevokedTokenRepository_DeleteExpiredPurgesOnlyExpired(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewRevokedTokenRepository(db)

	now := time.Now().UTC()
	require.NoError(t, repo.Create("jti-expired", now.Add(-time.Minute)), "Should denylist an already-expired token")
	require.NoError(t, repo.Create("jti-live", now.Add(time.Hour)), "Should denylist a live token")

	require.NoError(t, repo.DeleteExpired(), "Purge should succeed")

	expired, err := repo.IsRevoked("jti-expired")
	require.NoError(t, err, "Lookup after purge should not error")
	assert.False(t, expired, "The expired row must be purged")
	live, err := repo.IsRevoked("jti-live")
	require.NoError(t, err, "Lookup after purge should not error")
	assert.True(t, live, "The live row must survive the purge")
}
