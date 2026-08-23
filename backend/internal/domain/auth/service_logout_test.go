package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

// TestAuthService_Logout_RevokesOnlyPresentedSession asserts logout is
// per-session: two logins mint two sessions, logging out with one
// device's token deletes exactly that device's row and leaves the other
// device's session redeemable.
func TestAuthService_Logout_RevokesOnlyPresentedSession(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, deviceAToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in device A")
	_, deviceBToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in device B")
	require.Len(t, sessionRepo.SessionsForUser("user-testuser"), 2, "Two logins should leave two session rows")

	err = service.Logout(deviceAToken)

	assert.NoError(t, err, "Logging out a live session should not error")
	revokedRow, err := sessionRepo.GetByTokenHash(sha256Hex(deviceAToken))
	require.NoError(t, err, "Lookup of the logged-out hash should not error")
	assert.Nil(t, revokedRow, "The logged-out device's row must be deleted")
	survivingRow, err := sessionRepo.GetByTokenHash(sha256Hex(deviceBToken))
	require.NoError(t, err, "Lookup of the other device's hash should not error")
	assert.NotNil(t, survivingRow, "The other device's session must survive the logout")
}

// TestAuthService_Logout_UnknownTokenIsNoOp asserts logging out a token
// that matches no session row — never issued, or a second logout of an
// already-revoked session — is a silent success: logout's end state
// (no row) is already true, and idempotence forbids an error.
func TestAuthService_Logout_UnknownTokenIsNoOp(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in to mint a refresh token")
	require.NoError(t, service.Logout(loginToken), "First logout should succeed")

	assert.NoError(t, service.Logout(loginToken), "Second logout of the same token must not error")
	assert.NoError(t, service.Logout("never-issued-token"), "Logging out an unknown token must not error")
	assert.Empty(t, sessionRepo.SessionsForUser("user-testuser"), "No-op logouts must not disturb other rows")
}

// TestAuthService_Logout_StorageFailurePropagates asserts a session-store
// failure during logout is returned to the caller (the handler maps it
// to a 500) rather than swallowed into a false success.
func TestAuthService_Logout_StorageFailurePropagates(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	storageFailure := errors.New("session store unavailable")
	sessionRepo := testutils.NewMockSessionRepositoryWithErrors(nil, nil, storageFailure, nil)
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	err = service.Logout("any-token")

	assert.ErrorIs(t, err, storageFailure, "A storage failure during logout must propagate")
}
