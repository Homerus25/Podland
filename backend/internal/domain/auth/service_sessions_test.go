package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// sha256Hex is the test's independent implementation of the hash the
// service must persist: the expected value comes from the spec (SHA-256
// hex of the token), not from calling the service's own code.
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// TestAuthService_Login_CreatesHashedSession asserts login persists a
// session row for the authenticated user whose stored value is the
// SHA-256 hash of the returned refresh token — never the plaintext.
func TestAuthService_Login_CreatesHashedSession(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, refreshToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in with correct credentials")
	require.NotEmpty(t, refreshToken, "Login should return a refresh token")

	sessions := sessionRepo.SessionsForUser("user-testuser")
	require.Len(t, sessions, 1, "Login should create exactly one session row")

	stored := sessions[0]
	assert.NotEqual(t, refreshToken, stored.TokenHash, "Stored value must not be the plaintext token")
	assert.Equal(t, sha256Hex(refreshToken), stored.TokenHash, "Stored value must be the SHA-256 hex hash of the token")
	assert.Len(t, stored.TokenHash, 64, "SHA-256 hex hash is 64 characters")
	assert.Equal(t, "user-testuser", stored.UserID, "Session must belong to the authenticated user")
}

// TestAuthService_Login_RefreshTokenIs256BitBase64URL asserts generated
// refresh tokens decode to exactly 32 bytes (256 bits) of base64url
// without padding.
func TestAuthService_Login_RefreshTokenIs256BitBase64URL(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, refreshToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in with correct credentials")

	decoded, err := base64.RawURLEncoding.DecodeString(refreshToken)
	require.NoError(t, err, "Refresh token should be base64url without padding")
	assert.Len(t, decoded, 32, "Refresh token should decode to 256 bits")
}

// TestAuthService_Login_SessionExpiryUsesConfiguredTTL asserts the new
// session's expiry is the configured TTL away from creation.
func TestAuthService_Login_SessionExpiryUsesConfiguredTTL(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	ttl := 2 * time.Hour
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, ttl)
	require.NoError(t, err, "Should create auth service without error")

	before := time.Now()
	_, _, _, err = service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in with correct credentials")
	after := time.Now()

	sessions := sessionRepo.SessionsForUser("user-testuser")
	require.Len(t, sessions, 1, "Login should create exactly one session row")

	assert.True(t, !sessions[0].ExpiresAt.Before(before.Add(ttl)),
		"Expiry should be at least TTL after login (got %v, want >= %v)", sessions[0].ExpiresAt, before.Add(ttl))
	assert.True(t, !sessions[0].ExpiresAt.After(after.Add(ttl)),
		"Expiry should be at most TTL after login (got %v, want <= %v)", sessions[0].ExpiresAt, after.Add(ttl))
}

// TestAuthService_Login_EveryLoginCreatesItsOwnSession asserts the
// multi-session model: two logins leave two rows with distinct token
// hashes (laptop + phone stay logged in independently).
func TestAuthService_Login_EveryLoginCreatesItsOwnSession(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, firstToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "First login should succeed")
	_, secondToken, _, err := service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Second login should succeed")

	sessions := sessionRepo.SessionsForUser("user-testuser")
	assert.Len(t, sessions, 2, "Two logins should leave two session rows")
	assert.NotEqual(t, firstToken, secondToken, "Each login should get a distinct refresh token")

	firstHash := sha256Hex(firstToken)
	secondHash := sha256Hex(secondToken)
	for _, hash := range []string{firstHash, secondHash} {
		stored, err := sessionRepo.GetByTokenHash(hash)
		require.NoError(t, err, "Lookup by hash should not error")
		assert.NotNil(t, stored, "Each login's session should be findable by its hash")
	}
}

// TestAuthService_Login_PurgesExpiredSessions asserts the lazy cleanup:
// login deletes the user's already-expired rows while fresh ones survive.
func TestAuthService_Login_PurgesExpiredSessions(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionRepo.AddSession(&models.Session{
		ID:        "expired-session",
		UserID:    "user-testuser",
		TokenHash: "expired-hash",
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	freshSession := &models.Session{
		ID:        "fresh-session",
		UserID:    "user-testuser",
		TokenHash: "fresh-hash",
		CreatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(29 * 24 * time.Hour),
	}
	sessionRepo.AddSession(freshSession)

	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, _, _, err = service.Login("testuser", "test-password-123")
	require.NoError(t, err, "Should log in with correct credentials")

	gone, err := sessionRepo.GetByTokenHash("expired-hash")
	require.NoError(t, err, "Lookup of purged session should not error")
	assert.Nil(t, gone, "Expired session should be purged at login")

	kept, err := sessionRepo.GetByTokenHash("fresh-hash")
	require.NoError(t, err, "Lookup of fresh session should not error")
	assert.NotNil(t, kept, "Unexpired session should survive the purge")
}

// TestAuthService_Login_FailureCreatesNoSession asserts a rejected
// login never touches the session store.
func TestAuthService_Login_FailureCreatesNoSession(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, _, _, err = service.Login("testuser", "wrong-password")
	require.Error(t, err, "Wrong password should be rejected")

	assert.Empty(t, sessionRepo.SessionsForUser("user-testuser"), "No session should be created for a rejected login")
}

// TestAuthService_Login_SessionRepoErrorPropagates asserts a session
// storage failure surfaces as-is (mapped to 500 by the handler) rather
// than being swallowed into a login that silently has no session.
func TestAuthService_Login_SessionRepoErrorPropagates(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	dbError := assert.AnError
	sessionRepo := testutils.NewMockSessionRepositoryWithErrors(dbError, nil, nil, nil)
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, _, err := service.Login("testuser", "test-password-123")

	assert.ErrorIs(t, err, dbError, "Session creation failure should propagate unchanged")
	assert.Empty(t, accessToken, "No access token should be issued when the session cannot be persisted")
	assert.Empty(t, refreshToken, "No refresh token should be returned when the session cannot be persisted")
}
