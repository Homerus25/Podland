package auth

import (
	"errors"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mintRevocationToken mints a signed token whose jti and iat the test
// supplies, so revocation tests can place a token before or after a
// cutoff exactly. An empty jti reproduces a pre-change token (issue
// 441: those carry no jti and must skip the denylist check).
func mintRevocationToken(service *AuthService, userID, jti string, iat time.Time) (string, error) {
	return mintTokenWithClaims(service, userID, jwt.RegisteredClaims{
		ID:        jti,
		Issuer:    service.issuer,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(iat),
	})
}

// TestAuthService_ValidateToken_DenylistedJtiRejected asserts the
// denylist check: a cryptographically intact token whose jti is on the
// revoked_access_tokens table answers ErrTokenRevoked, not
// ErrTokenInvalid — the caller can tell a dead token from a forged one.
func TestAuthService_ValidateToken_DenylistedJtiRejected(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	token, err := service.GenerateToken("user-testuser")
	require.NoError(t, err, "Should mint a token")
	minted, err := service.ValidateToken(token)
	require.NoError(t, err, "The fresh token should validate")
	require.NoError(t, denylist.Create(minted.ID, time.Now().UTC().Add(time.Hour)), "Should denylist the jti")

	_, err = service.ValidateToken(token)

	assert.ErrorIs(t, err, apperrors.ErrTokenRevoked, "A denylisted jti must answer ErrTokenRevoked")
}

// TestAuthService_ValidateToken_PreCutoffIatRejected asserts the cutoff
// check: a token minted before the user's access-tokens-revoked-at stamp
// (the password-change/reset primitive, issue 441) answers
// ErrTokenRevoked even though its signature and expiry are intact.
func TestAuthService_ValidateToken_PreCutoffIatRejected(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	cutoff := time.Now().UTC().Truncate(time.Second)
	user, err := repo.GetByUsername("testuser")
	require.NoError(t, err, "Should read the seeded user")
	user.AccessTokensRevokedAt = &cutoff
	require.NoError(t, repo.Update(user), "Should stamp the cutoff")

	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, err := mintRevocationToken(service, user.ID, "jti-old", cutoff.Add(-time.Hour))
	require.NoError(t, err, "Should mint a pre-cutoff token")

	_, err = service.ValidateToken(token)

	assert.ErrorIs(t, err, apperrors.ErrTokenRevoked, "A pre-cutoff iat must answer ErrTokenRevoked")
}

// TestAuthService_ValidateToken_PostCutoffTokenAccepted asserts the
// cutoff's pass side: a token minted at or after the cutoff — the fresh
// token a password change hands back — still validates, so the cutoff
// kills exactly the older tokens, not the user's new login.
func TestAuthService_ValidateToken_PostCutoffTokenAccepted(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	cutoff := time.Now().UTC().Truncate(time.Second)
	user, err := repo.GetByUsername("testuser")
	require.NoError(t, err, "Should read the seeded user")
	user.AccessTokensRevokedAt = &cutoff
	require.NoError(t, repo.Update(user), "Should stamp the cutoff")

	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, err := mintRevocationToken(service, user.ID, "jti-fresh", cutoff)
	require.NoError(t, err, "Should mint a token at the cutoff")

	claims, err := service.ValidateToken(token)

	require.NoError(t, err, "A token minted at the cutoff must still validate")
	assert.Equal(t, user.ID, claims.UserID, "The accepted token should name its user")
}

// TestAuthService_ValidateToken_NoJtiSkipsDenylistCheck asserts the
// pre-change compatibility rule: tokens minted before issue 441 carry
// no jti, and the denylist check must skip them — there is nothing to
// key a denylist row on. The cutoff check still applies (it keys on the
// user, not the token).
func TestAuthService_ValidateToken_NoJtiSkipsDenylistCheck(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository().WithIsRevokedError(errors.New("denylist store unavailable"))
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	token, err := mintRevocationToken(service, "user-testuser", "", time.Now().UTC())
	require.NoError(t, err, "Should mint a jti-less token")

	_, err = service.ValidateToken(token)

	assert.NoError(t, err, "A jti-less token must skip the denylist check entirely")
}

// TestAuthService_ValidateToken_NilUserSkipsCutoffCheck asserts the
// deleted-user carve-out: a valid token whose user record no longer
// exists still validates (the status endpoint's documented behavior —
// it answers authenticated with no role), because there is no row left
// to carry a cutoff.
func TestAuthService_ValidateToken_NilUserSkipsCutoffCheck(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, err := mintRevocationToken(service, "user-deleted", "jti-gone", time.Now().UTC())
	require.NoError(t, err, "Should mint a token for a vanished user")

	claims, err := service.ValidateToken(token)

	require.NoError(t, err, "A token with no user record must still validate")
	assert.Equal(t, "user-deleted", claims.UserID, "The token should keep naming its (vanished) user")
}

// TestAuthService_ValidateToken_DenylistStorageErrorPropagates asserts
// a denylist-lookup storage failure is returned as-is: the JWT
// middleware's fail-closed fallback maps it to TOKEN_INVALID, so a
// broken store can never read as "not revoked".
func TestAuthService_ValidateToken_DenylistStorageErrorPropagates(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	storageFailure := errors.New("denylist store unavailable")
	denylist := testutils.NewMockRevokedTokenRepository().WithIsRevokedError(storageFailure)
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	token, err := service.GenerateToken("user-testuser")
	require.NoError(t, err, "Should mint a token")

	_, err = service.ValidateToken(token)

	assert.ErrorIs(t, err, storageFailure, "A denylist storage failure must propagate, not classify")
}

// TestAuthService_ValidateToken_UserLookupStorageErrorPropagates
// asserts a user-lookup storage failure during the cutoff check is
// returned as-is, mirroring the denylist: the middleware's fail-closed
// fallback answers TOKEN_INVALID.
func TestAuthService_ValidateToken_UserLookupStorageErrorPropagates(t *testing.T) {
	storageFailure := errors.New("user store unavailable")
	repo := testutils.NewMockUserRepositoryWithErrors(nil, storageFailure, nil, nil, nil, nil)
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, err := mintRevocationToken(service, "user-testuser", "jti-any", time.Now().UTC())
	require.NoError(t, err, "Should mint a token")

	_, err = service.ValidateToken(token)

	assert.ErrorIs(t, err, storageFailure, "A user-lookup storage failure must propagate, not classify")
}

// parseTokenClaims reads a token's claims through an independent
// parse — no revocation checks, no repository consults — so tests can
// read the jti and exp of a token before (or after) it is revoked.
func parseTokenClaims(service *AuthService, tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(*jwt.Token) (any, error) {
		return service.publicKey, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("token claims cast failed")
	}
	return claims, nil
}

// TestAuthService_Logout_DenylistsPresentedAccessToken asserts the
// per-device half of issue 441: logout revokes not just the refresh
// session behind the cookie but the access JWT presented in the
// Authorization header — its jti lands on the denylist with the
// token's own exp — so the old bearer answers ErrTokenRevoked on every
// protected endpoint while another device's token is untouched.
func TestAuthService_Logout_DenylistsPresentedAccessToken(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	sessionRepo := testutils.NewMockSessionRepository()
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithSessionRepo(repo, sessionRepo, 30*24*time.Hour)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	deviceAToken, deviceARefresh, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in device A")
	deviceBToken, _, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in device B")

	require.NoError(t, service.Logout(deviceARefresh, deviceAToken), "Logout with a presented bearer should succeed")

	_, err = service.ValidateToken(deviceAToken)
	assert.ErrorIs(t, err, apperrors.ErrTokenRevoked, "The logged-out bearer must answer ErrTokenRevoked")
	_, err = service.ValidateToken(deviceBToken)
	assert.NoError(t, err, "The other device's token must survive the logout")
}

// TestAuthService_Logout_DenylistKeysOnPresentedJti pins the denylist
// key precisely: the row logout writes is the presented token's jti,
// addressed by parsing the token before it is revoked.
func TestAuthService_Logout_DenylistKeysOnPresentedJti(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	accessToken, refreshToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in")

	// Read the jti and exp through an independent parse, before logout
	// revokes the token.
	parsed, err := parseTokenClaims(service, accessToken)
	require.NoError(t, err, "Should parse the minted token")

	require.NoError(t, service.Logout(refreshToken, accessToken), "Logout should succeed")

	revoked := denylist.Revoked()
	require.Len(t, revoked, 1, "Logout must denylist exactly one jti")
	expiry, denylisted := revoked[parsed.ID]
	assert.True(t, denylisted, "The denylist row must key on the presented token's jti")
	assert.Equal(t, parsed.ExpiresAt.Time.Unix(), expiry.Unix(), "The denylist row must carry the token's own exp")
}

// TestAuthService_Logout_WithoutAccessTokenStillSucceeds asserts the
// idempotent shape survives the new parameter: a logout with no bearer
// (or a bearer that fails validation) still succeeds and denylists
// nothing — there was no access token to kill.
func TestAuthService_Logout_WithoutAccessTokenStillSucceeds(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	accessToken, refreshToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in")

	assert.NoError(t, service.Logout(refreshToken, ""), "Logout without a bearer must still succeed")
	assert.NoError(t, service.Logout(refreshToken, "not-a-jwt"), "Logout with an invalid bearer must still succeed")
	assert.Empty(t, denylist.Revoked(), "Nothing was presentable to denylist")

	_, err = service.ValidateToken(accessToken)
	assert.NoError(t, err, "The access token must survive a logout that did not present it")
}

// TestAuthService_Logout_PreChangeTokenWithoutJtiNotDenylisted asserts
// the pre-change compatibility rule on the write side: a token with no
// jti cannot be denylisted (there is no key), and its logout is still
// a silent success.
func TestAuthService_Logout_PreChangeTokenWithoutJtiNotDenylisted(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	_, refreshToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in for the refresh token")
	preChange, err := mintRevocationToken(service, "user-testuser", "", time.Now().UTC())
	require.NoError(t, err, "Should mint a jti-less token")

	assert.NoError(t, service.Logout(refreshToken, preChange), "Logout with a jti-less bearer must still succeed")
	assert.Empty(t, denylist.Revoked(), "A jti-less token cannot be denylisted")
}

// TestAuthService_Logout_DenylistStorageFailurePropagates asserts a
// denylist-write failure during logout surfaces to the caller (the
// handler maps it to 500): a logout that failed to kill the presented
// access token must not report success.
func TestAuthService_Logout_DenylistStorageFailurePropagates(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	storageFailure := errors.New("denylist store unavailable")
	denylist := testutils.NewMockRevokedTokenRepository().WithCreateError(storageFailure)
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	accessToken, refreshToken, _, _, err := service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in")

	err = service.Logout(refreshToken, accessToken)

	assert.ErrorIs(t, err, storageFailure, "A denylist storage failure must propagate")
}

// TestAuthService_ChangePassword_SetsRevocationCutoff asserts the
// per-user half of issue 441 on the self-service path: a password
// change stamps users.access_tokens_revoked_at in the same user-row
// write that replaces the hash, so every access token minted before
// the change answers ErrTokenRevoked while the fresh token the change
// hands back — minted after the stamp — still validates.
func TestAuthService_ChangePassword_SetsRevocationCutoff(t *testing.T) {
	repo, _, service, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")

	// A token issued before the change: minted an hour ago, as every
	// real pre-change token was (a same-second token is not "older"
	// than the cutoff and correctly survives).
	oldToken, err := mintRevocationToken(service, userID, "jti-pre-change", time.Now().UTC().Add(-time.Hour))
	require.NoError(t, err, "Should mint a pre-change access token")

	freshToken, _, _, err := service.ChangePassword(userID, "old-password-123", "new-pass-8")
	require.NoError(t, err, "Password change should succeed")

	_, err = service.ValidateToken(oldToken)
	assert.ErrorIs(t, err, apperrors.ErrTokenRevoked, "A pre-change access token must answer ErrTokenRevoked")

	_, err = service.ValidateToken(freshToken)
	assert.NoError(t, err, "The fresh token minted after the cutoff must still validate")

	stored, err := repo.GetByID(userID)
	require.NoError(t, err, "Should read the user back")
	require.NotNil(t, stored.AccessTokensRevokedAt, "The password change must stamp the cutoff")
	assert.WithinDuration(t, time.Now().UTC(), *stored.AccessTokensRevokedAt, time.Minute,
		"The cutoff must be stamped at the change time")
	assert.Zero(t, stored.AccessTokensRevokedAt.UTC().Nanosecond(),
		"The cutoff must carry the shared stamp's second precision (User.RevokeAccessTokens, the jwt NumericDate granularity)")
}

// TestAuthService_Login_PurgesExpiredDenylistRows asserts the lazy
// cleanup (issue 441): login purges expired denylist rows — the same
// pattern as the sessions' DeleteExpiredForUser, no background
// sweeper — because a denylist row is dead weight once the token it
// addresses would fail validation anyway.
func TestAuthService_Login_PurgesExpiredDenylistRows(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	denylist := testutils.NewMockRevokedTokenRepository()
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	_, _, _, _, err = service.Login("testuser", "test-password-123", false)
	require.NoError(t, err, "Should log in")

	assert.Equal(t, 1, denylist.DeleteExpiredCalls(), "Login must purge expired denylist rows")
}

// TestAuthService_Login_DenylistPurgeFailurePropagates asserts a
// purge-time storage failure surfaces to the caller, mirroring the
// sessions purge's failure contract: login must not report success
// while the cleanup it owes silently did not happen.
func TestAuthService_Login_DenylistPurgeFailurePropagates(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	storageFailure := errors.New("denylist store unavailable")
	denylist := testutils.NewMockRevokedTokenRepository().WithDeleteExpiredError(storageFailure)
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")
	service = service.WithRevokedTokenRepo(denylist)

	_, _, _, _, err = service.Login("testuser", "test-password-123", false)

	assert.ErrorIs(t, err, storageFailure, "A denylist purge failure must propagate")
}
