package auth

import (
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthService_Refresh_UnknownTokenIsInvalid asserts a refresh token
// whose hash matches no session row is rejected as invalid — the same
// failure a replayed (already-rotated) token produces.
func TestAuthService_Refresh_UnknownTokenIsInvalid(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, user, _, err := service.Refresh("no-such-token")

	assert.ErrorIs(t, err, apperrors.ErrRefreshInvalid, "Unknown token must be rejected as invalid")
	assert.Empty(t, accessToken, "No access token should be issued for an unknown refresh token")
	assert.Empty(t, refreshToken, "No refresh token should be issued for an unknown refresh token")
	assert.Nil(t, user, "No user should be returned for an unknown refresh token")
}

// TestAuthService_Refresh_ValidTokenRotatesSession asserts the happy
// path end-to-end at the service seam: refresh returns a new access JWT
// for the session's user, the user themselves (so callers can restore
// display state without a second lookup), plus a distinct refresh
// token; and the session store reflects a rotation — old hash gone, new
// hash present, expiry slid forward by the full TTL.
func TestAuthService_Refresh_ValidTokenRotatesSession(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	ttl := 30 * 24 * time.Hour
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, ttl)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in to mint a refresh token")

	before := time.Now()
	accessToken, rotatedToken, user, _, err := service.Refresh(loginToken)
	require.NoError(t, err, "Valid refresh token should rotate")
	after := time.Now()

	assert.NotEmpty(t, accessToken, "Refresh should issue an access token")
	require.NotNil(t, user, "Refresh should return the session's user")
	assert.Equal(t, "testuser", user.Username, "Refresh should return the user the session belongs to")
	claims, err := service.ValidateToken(accessToken)
	require.NoError(t, err, "Issued access token should validate")
	assert.Equal(t, "user-testuser", claims.UserID, "Access token must belong to the session's user")

	require.NotEmpty(t, rotatedToken, "Refresh should return a rotated refresh token")
	assert.NotEqual(t, loginToken, rotatedToken, "Rotated refresh token must differ from the presented one")

	oldRow, err := sessionRepo.GetByTokenHash(sha256Hex(loginToken))
	require.NoError(t, err, "Lookup of rotated-out hash should not error")
	assert.Nil(t, oldRow, "Rotated-out hash must no longer address a session")

	newRow, err := sessionRepo.GetByTokenHash(sha256Hex(rotatedToken))
	require.NoError(t, err, "Lookup of rotated-in hash should not error")
	require.NotNil(t, newRow, "Rotated-in hash must address a session")
	assert.Equal(t, "user-testuser", newRow.UserID, "Rotated session must keep the session's user")
	assert.True(t, !newRow.ExpiresAt.Before(before.Add(ttl)),
		"Rotated expiry should be renewed to at least TTL after refresh (got %v, want >= %v)", newRow.ExpiresAt, before.Add(ttl))
	assert.True(t, !newRow.ExpiresAt.After(after.Add(ttl)),
		"Rotated expiry should be at most TTL after refresh (got %v, want <= %v)", newRow.ExpiresAt, after.Add(ttl))
}

// TestAuthService_Refresh_ReplayedTokenIsRejected asserts the rotation
// is single-use: presenting the token a second (already-consumed) time
// fails its lookup and yields the invalid-token error, not the expired
// one — the row is gone, not old.
func TestAuthService_Refresh_ReplayedTokenIsRejected(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in to mint a refresh token")

	_, firstRotation, user, _, err := service.Refresh(loginToken)
	require.NoError(t, err, "First refresh should succeed")
	require.NotEmpty(t, firstRotation, "First refresh should rotate the token")
	require.NotNil(t, user, "First refresh should return the session's user")

	accessToken, replayToken, user, _, err := service.Refresh(loginToken)

	assert.ErrorIs(t, err, apperrors.ErrRefreshInvalid, "Replaying the consumed token must be rejected as invalid")
	assert.NotErrorIs(t, err, apperrors.ErrSessionExpired, "A replay is a missing row, not an expired one")
	assert.Empty(t, accessToken, "No access token should be issued for a replay")
	assert.Empty(t, replayToken, "No refresh token should be issued for a replay")
	assert.Nil(t, user, "No user should be returned for a replay")
}

// TestAuthService_Refresh_ExpiredSessionFailsAndIsDeleted asserts a
// found-but-expired row yields the session-expired error and is deleted
// on encounter — the lazy cleanup that keeps the table from filling with
// dead rows without a background sweeper.
func TestAuthService_Refresh_ExpiredSessionFailsAndIsDeleted(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	expiredHash := sha256Hex("expired-refresh-token")
	sessionRepo.AddSession(&models.Session{
		ID:        "expired-session",
		UserID:    "user-testuser",
		TokenHash: expiredHash,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, user, _, err := service.Refresh("expired-refresh-token")

	assert.ErrorIs(t, err, apperrors.ErrSessionExpired, "Expired session must report the session-expired error")
	assert.Empty(t, accessToken, "No access token should be issued for an expired session")
	assert.Empty(t, refreshToken, "No refresh token should be issued for an expired session")
	assert.Nil(t, user, "No user should be returned for an expired session")

	gone, err := sessionRepo.GetByTokenHash(expiredHash)
	require.NoError(t, err, "Lookup of the expired row should not error")
	assert.Nil(t, gone, "Expired session row should be deleted on encounter")
}

// losingRaceSessionRepo wraps a mock session repository so the row the
// service just looked up disappears before rotation — exactly the state
// a concurrent refresh with the same cookie leaves behind. It forces the
// Rotate-returns-false path deterministically, no goroutines needed.
type losingRaceSessionRepo struct {
	models.SessionRepositoryInterface
}

func (l *losingRaceSessionRepo) Rotate(oldTokenHash string, newSession *models.Session) (bool, error) {
	l.SessionRepositoryInterface.DeleteByTokenHash(oldTokenHash)
	return l.SessionRepositoryInterface.Rotate(oldTokenHash, newSession)
}

// TestAuthService_Refresh_LostRotationRaceIsCleanInvalid asserts that
// when the atomic rotation loses to a concurrent refresh (the row
// vanished between lookup and rotate), the caller gets the same clean
// invalid-token failure as any other unknown token — never a partial
// success and never a duplicate session.
func TestAuthService_Refresh_LostRotationRaceIsCleanInvalid(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	_, loginToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in to mint a refresh token")

	raceRepo := &losingRaceSessionRepo{SessionRepositoryInterface: sessionRepo}
	raceService, err := NewTestAuthServiceWithSessionRepo(repo, raceRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create racing auth service without error")

	accessToken, refreshToken, user, _, err := raceService.Refresh(loginToken)

	assert.ErrorIs(t, err, apperrors.ErrRefreshInvalid, "Losing the rotation race must surface as invalid")
	assert.Empty(t, accessToken, "No access token should be issued when the race is lost")
	assert.Empty(t, refreshToken, "No refresh token should be issued when the race is lost")
	assert.Nil(t, user, "No user should be returned when the race is lost")
	assert.Empty(t, sessionRepo.SessionsForUser("user-testuser"), "A lost race must not leave a rotated row behind")
}

// TestAuthService_Refresh_OrphanedSessionIsInvalid asserts a session row
// whose user no longer exists is rejected as invalid — a deleted user
// leaves the cookie meaningless, indistinguishable from any other
// unknown token, and the caller is sent to re-login rather than held in
// a half-authenticated state.
func TestAuthService_Refresh_OrphanedSessionIsInvalid(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	sessionRepo.AddSession(&models.Session{
		ID:        "orphaned-session",
		UserID:    "user-deleted",
		TokenHash: sha256Hex("orphaned-cookie-token"),
		CreatedAt: time.Now().Add(-1 * time.Hour),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")

	accessToken, refreshToken, user, _, err := service.Refresh("orphaned-cookie-token")

	assert.ErrorIs(t, err, apperrors.ErrRefreshInvalid, "An orphaned session must be rejected as invalid")
	assert.Empty(t, accessToken, "No access token should be issued for an orphaned session")
	assert.Empty(t, refreshToken, "No refresh token should be issued for an orphaned session")
	assert.Nil(t, user, "No user should be returned for an orphaned session")
}
