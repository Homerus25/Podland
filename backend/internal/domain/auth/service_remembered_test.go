package auth

import (
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The remembered-session contract (issue 379): a login that opts in via
// rememberDevice produces a session row flagged remembered with expiry
// on the remembered TTL, sliding on every refresh with the flag carried
// onto the replacement row; a login without the flag behaves exactly as
// today. The service also reports the Max-Age the refresh cookie should
// carry — the full remembered TTL for a remembered session, 0 (a
// browser-session cookie) otherwise — so the cookie slides in lockstep
// with the row. These tests pin the two TTLs to distinct values so the
// arithmetic cannot pass by coincidence.

// TestAuthService_Login_RememberedDeviceFlagsSessionAndUsesRememberedTTL
// asserts a rememberDevice login persists a remembered session row whose
// expiry is the remembered TTL from now, and answers the remembered TTL
// as the refresh cookie's Max-Age.
func TestAuthService_Login_RememberedDeviceFlagsSessionAndUsesRememberedTTL(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionTTL := 30 * 24 * time.Hour
	rememberTTL := 90 * 24 * time.Hour
	service, err := NewTestAuthServiceWithRememberTTL(repo, sessionRepo, sessionTTL, rememberTTL)
	require.NoError(t, err, "Should create auth service without error")

	before := time.Now()
	_, refreshToken, _, cookieMaxAge, err := service.Login("testuser", "test-password-123", true)
	require.NoError(t, err, "Remembered login should succeed")
	after := time.Now()

	sessions := sessionRepo.SessionsForUser("user-testuser")
	require.Len(t, sessions, 1, "Remembered login should create exactly one session row")

	stored := sessions[0]
	assert.Equal(t, sha256Hex(refreshToken), stored.TokenHash, "Remembered session must key on the token's hash")
	assert.True(t, stored.Remembered, "Remembered login must flag the session row remembered")
	assert.True(t, !stored.ExpiresAt.Before(before.Add(rememberTTL)),
		"Remembered expiry should be at least the remembered TTL after login (got %v, want >= %v)", stored.ExpiresAt, before.Add(rememberTTL))
	assert.True(t, !stored.ExpiresAt.After(after.Add(rememberTTL)),
		"Remembered expiry should be at most the remembered TTL after login (got %v, want <= %v)", stored.ExpiresAt, after.Add(rememberTTL))

	assert.Equal(t, rememberTTL, cookieMaxAge, "Remembered login must answer the remembered TTL as the refresh cookie's Max-Age")
}

// TestAuthService_Login_WithoutRememberDeviceIsUnchanged asserts a login
// without the flag behaves exactly as before the remembered-session
// feature: an unremembered row on the session TTL, and no cookie
// Max-Age (a browser-session cookie).
func TestAuthService_Login_WithoutRememberDeviceIsUnchanged(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionTTL := 30 * 24 * time.Hour
	rememberTTL := 90 * 24 * time.Hour
	service, err := NewTestAuthServiceWithRememberTTL(repo, sessionRepo, sessionTTL, rememberTTL)
	require.NoError(t, err, "Should create auth service without error")

	before := time.Now()
	_, _, _, cookieMaxAge, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Plain login should succeed")
	after := time.Now()

	sessions := sessionRepo.SessionsForUser("user-testuser")
	require.Len(t, sessions, 1, "Plain login should create exactly one session row")

	stored := sessions[0]
	assert.False(t, stored.Remembered, "Plain login must leave the session row unremembered")
	assert.True(t, !stored.ExpiresAt.Before(before.Add(sessionTTL)),
		"Plain expiry should be at least the session TTL after login (got %v, want >= %v)", stored.ExpiresAt, before.Add(sessionTTL))
	assert.True(t, !stored.ExpiresAt.After(after.Add(sessionTTL)),
		"Plain expiry should be at most the session TTL after login (got %v, want <= %v)", stored.ExpiresAt, after.Add(sessionTTL))

	assert.Zero(t, cookieMaxAge, "Plain login must answer no cookie Max-Age (a browser-session cookie)")
}

// TestAuthService_Refresh_RememberedSessionSlidesOnRememberedTTL asserts
// refreshing a remembered session renews the expiry by the remembered
// TTL, carries the remembered flag onto the replacement row, and
// re-answers the full remembered TTL as the cookie's Max-Age.
func TestAuthService_Refresh_RememberedSessionSlidesOnRememberedTTL(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionTTL := 30 * 24 * time.Hour
	rememberTTL := 90 * 24 * time.Hour
	service, err := NewTestAuthServiceWithRememberTTL(repo, sessionRepo, sessionTTL, rememberTTL)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", true)
	require.NoError(t, err, "Remembered login should succeed")

	before := time.Now()
	_, rotatedToken, _, cookieMaxAge, err := service.Refresh(loginToken)
	require.NoError(t, err, "Refreshing a remembered session should succeed")
	after := time.Now()

	newRow, err := sessionRepo.GetByTokenHash(sha256Hex(rotatedToken))
	require.NoError(t, err, "Lookup of rotated-in hash should not error")
	require.NotNil(t, newRow, "Rotated-in hash must address a session")
	assert.True(t, newRow.Remembered, "Rotation must carry the remembered flag onto the replacement row")
	assert.True(t, !newRow.ExpiresAt.Before(before.Add(rememberTTL)),
		"Remembered refresh must renew the expiry by the remembered TTL (got %v, want >= %v)", newRow.ExpiresAt, before.Add(rememberTTL))
	assert.True(t, !newRow.ExpiresAt.After(after.Add(rememberTTL)),
		"Remembered refresh must renew the expiry by the remembered TTL (got %v, want <= %v)", newRow.ExpiresAt, after.Add(rememberTTL))

	assert.Equal(t, rememberTTL, cookieMaxAge, "Remembered refresh must re-answer the full remembered TTL as the cookie's Max-Age")
}

// TestAuthService_Refresh_UnrememberedSessionSlidesOnSessionTTL asserts
// refreshing a plain session keeps today's behavior: the session TTL's
// sliding renewal, an unremembered replacement row, and no cookie
// Max-Age.
func TestAuthService_Refresh_UnrememberedSessionSlidesOnSessionTTL(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionTTL := 30 * 24 * time.Hour
	rememberTTL := 90 * 24 * time.Hour
	service, err := NewTestAuthServiceWithRememberTTL(repo, sessionRepo, sessionTTL, rememberTTL)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Plain login should succeed")

	before := time.Now()
	_, rotatedToken, _, cookieMaxAge, err := service.Refresh(loginToken)
	require.NoError(t, err, "Refreshing a plain session should succeed")
	after := time.Now()

	newRow, err := sessionRepo.GetByTokenHash(sha256Hex(rotatedToken))
	require.NoError(t, err, "Lookup of rotated-in hash should not error")
	require.NotNil(t, newRow, "Rotated-in hash must address a session")
	assert.False(t, newRow.Remembered, "Plain refresh must keep the replacement row unremembered")
	assert.True(t, !newRow.ExpiresAt.Before(before.Add(sessionTTL)),
		"Plain refresh must renew the expiry by the session TTL (got %v, want >= %v)", newRow.ExpiresAt, before.Add(sessionTTL))
	assert.True(t, !newRow.ExpiresAt.After(after.Add(sessionTTL)),
		"Plain refresh must renew the expiry by the session TTL (got %v, want <= %v)", newRow.ExpiresAt, after.Add(sessionTTL))

	assert.Zero(t, cookieMaxAge, "Plain refresh must answer no cookie Max-Age")
}
