package users

import (
	"fmt"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// passwordResetFixture seeds an admin and two users, all three
// carrying a real bcrypt hash of the same known password, plus two
// live sessions for alice and one each for bob and the admin, so
// password-reset tests can assert exactly whose sessions survive. It
// returns the service wired over both mock repositories.
func passwordResetFixture(t *testing.T) (*UserService, *testutils.MockUserRepository, *testutils.MockSessionRepository) {
	t.Helper()

	repo := testutils.NewMockUserRepository()
	sessionRepo := testutils.NewMockSessionRepository()
	service := NewUserService(repo, sessionRepo)

	knownHash, err := bcrypt.GenerateFromPassword([]byte("known-old-password"), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash the known password")

	created := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	repo.AddUser(&models.User{ID: "admin-1", Username: "root", Role: models.RoleAdmin, PasswordHash: string(knownHash), CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-1", Username: "alice", Role: models.RoleUser, PasswordHash: string(knownHash), CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-2", Username: "bob", Role: models.RoleUser, PasswordHash: string(knownHash), CreatedAt: created})

	sessionRepo.AddSession(&models.Session{ID: "s-1", UserID: "user-1", TokenHash: "alice-hash-1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	sessionRepo.AddSession(&models.Session{ID: "s-2", UserID: "user-1", TokenHash: "alice-hash-2", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	sessionRepo.AddSession(&models.Session{ID: "s-3", UserID: "user-2", TokenHash: "bob-hash-1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})
	sessionRepo.AddSession(&models.Session{ID: "s-4", UserID: "admin-1", TokenHash: "admin-hash-1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)})

	return service, repo, sessionRepo
}

// storedPasswordMatches reports whether the user's stored hash
// authenticates the given plaintext password.
func storedPasswordMatches(t *testing.T, repo *testutils.MockUserRepository, userID, password string) bool {
	t.Helper()

	user, err := repo.GetByID(userID)
	require.NoError(t, err, "Should read the user back from the repository")
	require.NotNil(t, user, "User %q should exist in the repository", userID)
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
}

// TestUserService_ResetPassword_Success asserts the full contract: the
// hash is replaced so the new password authenticates and the old one
// dies, every session of the target user is revoked, and everyone
// else's sessions — the admin's included — are untouched. No fresh
// login is minted for the target: unlike the self-service change, the
// caller is the admin, and the target must log in with the new
// password.
func TestUserService_ResetPassword_Success(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)

	reset, err := service.ResetPassword("user-1", "brand-new-password")
	require.NoError(t, err, "An admin reset should succeed")
	require.NotNil(t, reset, "The updated user should be returned")
	assert.Equal(t, "user-1", reset.ID, "The updated user should be the target")
	assert.Equal(t, "alice", reset.Username, "The target's username should ride through")

	assert.False(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The old password must no longer authenticate")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "brand-new-password"), "The new password must authenticate")

	assert.Empty(t, sessionRepo.SessionsForUser("user-1"), "Every session of the target must be revoked")
	assert.Len(t, sessionRepo.SessionsForUser("user-2"), 1, "Other users' sessions must be untouched")
	assert.Len(t, sessionRepo.SessionsForUser("admin-1"), 1, "The admin's own session must be untouched")
}

// TestUserService_ResetPassword_AdminSelfReset pins the caller ==
// target edge of the revoke-all contract: an admin resetting their own
// password is an ordinary reset whose target happens to be the caller,
// so the admin's own sessions are revoked exactly like any other
// target's — the story's "all of that user's sessions" has no caller
// exception. The new password authenticates, the old one dies, and the
// other users' sessions are untouched.
func TestUserService_ResetPassword_AdminSelfReset(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)

	reset, err := service.ResetPassword("admin-1", "brand-new-password")
	require.NoError(t, err, "An admin resetting their own password should succeed")
	require.NotNil(t, reset, "The updated user should be returned")
	assert.Equal(t, "admin-1", reset.ID, "The updated user should be the caller themself")
	assert.Equal(t, models.RoleAdmin, reset.Role, "The admin role must ride through unchanged")

	assert.False(t, storedPasswordMatches(t, repo, "admin-1", "known-old-password"), "The old password must no longer authenticate")
	assert.True(t, storedPasswordMatches(t, repo, "admin-1", "brand-new-password"), "The new password must authenticate")

	assert.Empty(t, sessionRepo.SessionsForUser("admin-1"), "Every session of the admin themself must be revoked")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Other users' sessions must be untouched")
	assert.Len(t, sessionRepo.SessionsForUser("user-2"), 1, "Other users' sessions must be untouched")
}

// TestUserService_ResetPassword_BoundaryLengthAccepted pins the policy
// floor: a password of exactly the minimum length is accepted.
func TestUserService_ResetPassword_BoundaryLengthAccepted(t *testing.T) {
	service, repo, _ := passwordResetFixture(t)

	_, err := service.ResetPassword("user-1", "12345678")
	require.NoError(t, err, "A password of exactly the minimum length should be accepted")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "12345678"), "The boundary-length password should be persisted")
}

// TestUserService_ResetPassword_NewPasswordTooShort asserts the policy:
// a new password below models.MinPasswordLength is rejected with
// ErrPasswordTooShort and nothing changes — not the hash, not the
// sessions.
func TestUserService_ResetPassword_NewPasswordTooShort(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)

	reset, err := service.ResetPassword("user-1", "short7")

	require.ErrorIs(t, err, apperrors.ErrPasswordTooShort, "A 7-character password is below the floor")
	assert.Nil(t, reset, "No user is returned for a rejected reset")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The password must be unchanged")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Sessions must be untouched")
}

// TestUserService_ResetPassword_LengthCountsCharactersNotBytes pins the
// policy's unit: the floor is characters, so a multibyte password with
// fewer characters than the floor is rejected even when its byte length
// clears it.
func TestUserService_ResetPassword_LengthCountsCharactersNotBytes(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)

	_, err := service.ResetPassword("user-1", "日本語日本")

	require.ErrorIs(t, err, apperrors.ErrPasswordTooShort, "Four characters is below the floor despite twelve bytes")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The password must be unchanged")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "Sessions must be untouched")
}

// TestUserService_ResetPassword_TargetNotFound asserts resetting the
// password of a user that does not exist is answered with
// ErrUserNotFound and nothing is written.
func TestUserService_ResetPassword_TargetNotFound(t *testing.T) {
	service, _, sessionRepo := passwordResetFixture(t)

	reset, err := service.ResetPassword("ghost-id", "brand-new-password")

	require.ErrorIs(t, err, apperrors.ErrUserNotFound, "A missing target user should surface USER_NOT_FOUND")
	assert.Nil(t, reset, "No user is returned for a missing target")
	assert.Empty(t, sessionRepo.SessionsForUser("ghost-id"), "A missing target has no sessions to revoke")
}

// TestUserService_ResetPassword_UserRepoErrorPropagates asserts a hash
// write failure surfaces unchanged — no revocation is undone for a
// reset that did not happen. Because the revocation runs before the
// hash write, the failure leaves the target signed out everywhere with
// the old password still working: the safe side of "signed out but not
// changed".
func TestUserService_ResetPassword_UserRepoErrorPropagates(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)
	repo.WithUpdateError(fmt.Errorf("disk on fire"))

	reset, err := service.ResetPassword("user-1", "brand-new-password")

	require.Error(t, err, "A failing update must fail the call")
	assert.Nil(t, reset, "No user is returned for a failed reset")
	assert.Contains(t, err.Error(), "disk on fire", "The repository cause should propagate")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The password must be unchanged")
	assert.Empty(t, sessionRepo.SessionsForUser("user-1"), "The revocation that already ran must not be undone")
}

// TestUserService_ResetPassword_RevokeErrorPropagates asserts a
// revoke-all failure surfaces unchanged and nothing is written: the
// revocation runs before the hash write, so the password is unchanged
// and the target's sessions stand — a stolen token must not outlive
// the reset — and the retry re-runs the idempotent revocation before
// the hash write.
func TestUserService_ResetPassword_RevokeErrorPropagates(t *testing.T) {
	service, repo, sessionRepo := passwordResetFixture(t)
	sessionRepo.WithDeleteAllError(fmt.Errorf("revoke failed"))

	reset, err := service.ResetPassword("user-1", "brand-new-password")

	require.Error(t, err, "A failing revoke-all must fail the call")
	assert.Nil(t, reset, "No user is returned for a failed reset")
	assert.Contains(t, err.Error(), "revoke failed", "The repository cause should propagate")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The hash write must not run when revocation fails")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "A failed revocation must leave the sessions standing")
}
