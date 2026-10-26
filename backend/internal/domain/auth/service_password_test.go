package auth

import (
	"net/http"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// seedPasswordChangeUser seeds a user (bcrypt-hashed) and logs two
// sessions in for them, so password-change tests can assert on the
// revocation of pre-existing sessions. It returns the user repo, the
// session repo, a service built over both, and the user's ID.
func seedPasswordChangeUser(t *testing.T, username, currentPassword string) (*testutils.MockUserRepository, *testutils.MockSessionRepository, *AuthService, string) {
	t.Helper()

	repo := seedLoginUser(t, username, currentPassword)
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	for range 2 {
		_, _, _, _, err := service.Login(username, currentPassword, false)
		require.NoError(t, err, "Should log in to seed a session")
	}
	require.Len(t, sessionRepo.SessionsForUser("user-"+username), 2, "Two seeded logins should leave two sessions")

	return repo, sessionRepo, service, "user-" + username
}

// passwordMatches reports whether the user's stored hash authenticates
// the given plaintext password.
func passwordMatches(t *testing.T, repo *testutils.MockUserRepository, userID, password string) bool {
	t.Helper()

	user, err := repo.GetByID(userID)
	require.NoError(t, err, "Should look up the user")
	require.NotNil(t, user, "User should exist")
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

// TestAuthService_ChangePassword_Success asserts the full contract: the
// current password verifies, the new password meets the policy floor,
// the hash is replaced, every pre-existing session is revoked, and a
// fresh login comes back (access JWT for the user + refresh token whose
// session is live) so the changing device stays signed in.
func TestAuthService_ChangePassword_Success(t *testing.T) {
	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	accessToken, refreshToken, user, err := service.ChangePassword(userID, "old-password-123", "new-pass-8")

	require.NoError(t, err, "Password change with correct current password should succeed")
	require.NotNil(t, user, "Should return the user")
	assert.Equal(t, userID, user.ID, "Should return the changed user")

	// Fresh login: the access token validates and names the user.
	claims, err := service.ValidateToken(accessToken)
	require.NoError(t, err, "Returned access token should validate")
	assert.Equal(t, userID, claims.UserID, "Access token should be for the changing user")

	// The refresh token addresses exactly one live session — the new
	// one; both pre-existing sessions are gone.
	sessions := sessionRepo.SessionsForUser(userID)
	require.Len(t, sessions, 1, "The change should leave exactly one live session (the fresh login)")
	assert.Equal(t, sha256Hex(refreshToken), sessions[0].TokenHash, "The surviving session is the fresh login's")
	session, err := sessionRepo.GetByTokenHash(sha256Hex(refreshToken))
	require.NoError(t, err, "Fresh refresh token should resolve")
	require.NotNil(t, session, "Fresh refresh token should address a live session")

	// The hash swap took effect: old password dies, new one works.
	assert.False(t, passwordMatches(t, repo, userID, "old-password-123"), "Old password must no longer authenticate")
	assert.True(t, passwordMatches(t, repo, userID, "new-pass-8"), "New password must authenticate")
}

// TestAuthService_ChangePassword_RevokesRememberedSessionsToo asserts
// the revocation is unconditional (issue 379): a remembered session —
// the long-lived, persistent-cookie kind — dies with the password
// change like every other one, so a compromised password cannot be
// ridden by remembered devices. The fresh login the change mints is
// unremembered, exactly as today.
func TestAuthService_ChangePassword_RevokesRememberedSessionsToo(t *testing.T) {
	_, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	_, rememberedToken, _, _, err := service.Login("testuser", "old-password-123", true)
	require.NoError(t, err, "Remembered login should succeed")

	_, _, _, err = service.ChangePassword(userID, "old-password-123", "new-pass-8")
	require.NoError(t, err, "Password change should succeed")

	remembered, err := sessionRepo.GetByTokenHash(sha256Hex(rememberedToken))
	require.NoError(t, err, "Remembered-session lookup should not error")
	assert.Nil(t, remembered, "The remembered session must be revoked with every other one")

	sessions := sessionRepo.SessionsForUser(userID)
	require.Len(t, sessions, 1, "The change should leave exactly one live session (the fresh login)")
	assert.False(t, sessions[0].Remembered, "The fresh login the change mints must be unremembered")
}

// TestAuthService_ChangePassword_WrongCurrentPassword asserts the
// authority check: a wrong current password is rejected with a clear
// 401 error and nothing changes — not the hash, not the sessions.
func TestAuthService_ChangePassword_WrongCurrentPassword(t *testing.T) {
	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	accessToken, refreshToken, _, err := service.ChangePassword(userID, "wrong-current", "new-pass-8")

	// WithMessage returns a copy (AppError has no Is), so the sentinel
	// identity is pinned via its code: 401 INVALID_CREDENTIALS with a
	// message that names the current password as the problem.
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok, "Failure should be an AppError")
	assert.Equal(t, "INVALID_CREDENTIALS", appErr.Code, "Wrong current password should carry the credentials code")
	assert.Equal(t, http.StatusUnauthorized, appErr.Status, "Wrong current password should be a 401")
	assert.Equal(t, "Current password is incorrect", appErr.Message, "The error should name the current password as the problem")
	assert.Empty(t, accessToken, "No access token on rejection")
	assert.Empty(t, refreshToken, "No refresh token on rejection")
	assert.True(t, passwordMatches(t, repo, userID, "old-password-123"), "Password must be unchanged")
	assert.Len(t, sessionRepo.SessionsForUser(userID), 2, "Sessions must be untouched")
}

// TestAuthService_ChangePassword_NewPasswordTooShort asserts the
// password policy: a new password below models.MinPasswordLength is
// rejected with ErrPasswordTooShort and nothing changes.
func TestAuthService_ChangePassword_NewPasswordTooShort(t *testing.T) {
	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	accessToken, refreshToken, _, err := service.ChangePassword(userID, "old-password-123", "short7")

	require.ErrorIs(t, err, apperrors.ErrPasswordTooShort, "A 7-character password is below the floor")
	assert.Empty(t, accessToken, "No access token on rejection")
	assert.Empty(t, refreshToken, "No refresh token on rejection")
	assert.True(t, passwordMatches(t, repo, userID, "old-password-123"), "Password must be unchanged")
	assert.Len(t, sessionRepo.SessionsForUser(userID), 2, "Sessions must be untouched")
}

// TestAuthService_ChangePassword_LengthCountsCharactersNotBytes pins the
// policy's unit: the floor is characters, so a multibyte password with
// fewer characters than the floor is rejected even when its byte length
// clears it.
func TestAuthService_ChangePassword_LengthCountsCharactersNotBytes(t *testing.T) {
	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	_, _, _, err := service.ChangePassword(userID, "old-password-123", "日本語日本")

	require.ErrorIs(t, err, apperrors.ErrPasswordTooShort, "Four characters is below the floor despite twelve bytes")
	assert.True(t, passwordMatches(t, repo, userID, "old-password-123"), "Password must be unchanged")
	assert.Len(t, sessionRepo.SessionsForUser(userID), 2, "Sessions must be untouched")
}

// TestAuthService_ChangePassword_UnknownUser asserts a caller whose
// user record no longer exists is answered with ErrUserNotFound.
func TestAuthService_ChangePassword_UnknownUser(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, _, _, err = service.ChangePassword("no-such-user", "old-password-123", "new-pass-8")

	require.ErrorIs(t, err, apperrors.ErrUserNotFound, "A missing user record should surface as USER_NOT_FOUND")
}

// TestAuthService_ChangePassword_BoundaryLengthAccepted pins the policy
// floor on the self-service path: a new password of exactly the minimum
// length is accepted and replaces the hash.
func TestAuthService_ChangePassword_BoundaryLengthAccepted(t *testing.T) {
	repo, _, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	_, _, _, err := service.ChangePassword(userID, "old-password-123", "12345678")

	require.NoError(t, err, "A password of exactly the minimum length should be accepted")
	assert.False(t, passwordMatches(t, repo, userID, "old-password-123"), "The old password must no longer authenticate")
	assert.True(t, passwordMatches(t, repo, userID, "12345678"), "The boundary-length password should be persisted")
}

// TestAuthService_ChangePassword_UserRepoErrorPropagates asserts a hash
// write failure surfaces unchanged — no fresh login is minted for a
// change that did not happen. Because the revocation runs before the
// hash write, the failure leaves the caller signed out everywhere with
// the old password still working: the safe side of "signed out but not
// changed".
func TestAuthService_ChangePassword_UserRepoErrorPropagates(t *testing.T) {
	repo, sessionRepo, _, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")
	repo.WithUpdateError(assert.AnError)
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, _, err := service.ChangePassword(userID, "old-password-123", "new-pass-8")

	assert.ErrorIs(t, err, assert.AnError, "Update failure should propagate unchanged")
	assert.Empty(t, accessToken, "No access token on failure")
	assert.Empty(t, refreshToken, "No refresh token on failure")
	assert.True(t, passwordMatches(t, repo, userID, "old-password-123"), "The password must be unchanged")
	assert.Empty(t, sessionRepo.SessionsForUser(userID), "The revocation that already ran must not be undone")
}

// TestAuthService_ChangePassword_RevokeErrorPropagates asserts a
// revoke-all failure surfaces unchanged and nothing is written: the
// revocation runs before the hash write, so the password is unchanged
// and the pre-existing sessions stand — a stolen token must not outlive
// the change — and no fresh session is issued for a change that did
// not happen.
func TestAuthService_ChangePassword_RevokeErrorPropagates(t *testing.T) {
	repo, sessionRepo, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")
	sessionRepo.WithDeleteAllError(assert.AnError)
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, _, err := service.ChangePassword(userID, "old-password-123", "new-pass-8")

	assert.ErrorIs(t, err, assert.AnError, "Revoke-all failure should propagate unchanged")
	assert.Empty(t, accessToken, "No access token on failure")
	assert.Empty(t, refreshToken, "No refresh token on failure")
	assert.True(t, passwordMatches(t, repo, userID, "old-password-123"), "The hash write must not run when revocation fails")
	assert.Len(t, sessionRepo.SessionsForUser(userID), 2, "No fresh session may be issued when revocation fails")
}
