package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUser_RevokeAccessTokens pins the revocation-cutoff stamping
// ritual (issue 441) at its one owner: both password paths — the
// self-service change (auth.ChangePassword) and the admin reset
// (users.ResetPassword) — call this method, so the stamp's shape cannot
// drift between them. The contract: the cutoff becomes non-nil, UTC,
// truncated to whole seconds — the jwt NumericDate granularity — so a
// token minted after the call (iat >= cutoff) passes validation while
// every older token fails it.
func TestUser_RevokeAccessTokens(t *testing.T) {
	user := &User{ID: "user-1", Username: "testuser"}

	user.RevokeAccessTokens()

	require.NotNil(t, user.AccessTokensRevokedAt, "The stamp must set the cutoff")
	cutoff := *user.AccessTokensRevokedAt
	assert.Equal(t, time.UTC, cutoff.Location(), "The cutoff must be UTC")
	assert.Zero(t, cutoff.Nanosecond(), "The cutoff must be truncated to whole seconds (the jwt NumericDate granularity)")
	assert.WithinDuration(t, time.Now().UTC(), cutoff, time.Minute, "The cutoff must be stamped at call time")
}

// TestUser_RevokeAccessTokens_OverwritesOldStamp pins the re-stamp: a
// user whose cutoff was already stamped (an earlier password change)
// gets a fresh cutoff on the next call, never the stale one — a second
// password change must not leave the first change's cutoff standing.
func TestUser_RevokeAccessTokens_OverwritesOldStamp(t *testing.T) {
	stale := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	user := &User{ID: "user-1", Username: "testuser", AccessTokensRevokedAt: &stale}

	user.RevokeAccessTokens()

	require.NotNil(t, user.AccessTokensRevokedAt, "The re-stamp must set the cutoff")
	assert.True(t, user.AccessTokensRevokedAt.After(stale), "The re-stamp must replace the stale cutoff with a fresh one")
}

// TestUser_RevokeAccessTokens_IndependentStamps pins pointer
// independence: stamping one user must not alias another user's cutoff —
// each stamp writes its own value, so a later mutation of one user's
// cutoff cannot move another's.
func TestUser_RevokeAccessTokens_IndependentStamps(t *testing.T) {
	first := &User{ID: "user-1", Username: "first"}
	second := &User{ID: "user-2", Username: "second"}

	first.RevokeAccessTokens()
	second.RevokeAccessTokens()

	require.NotNil(t, first.AccessTokensRevokedAt, "The first user must be stamped")
	require.NotNil(t, second.AccessTokensRevokedAt, "The second user must be stamped")
	moved := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	*first.AccessTokensRevokedAt = moved
	assert.NotEqual(t, moved, *second.AccessTokensRevokedAt, "The stamps must be independent values, not aliases")
}
