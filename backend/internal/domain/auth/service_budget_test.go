package auth

import (
	"errors"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBudget is the failurebudget.Brake seam's function-field fake: a
// fixed-count budget per key without the clock. Record spends until the
// budget is gone (further records are no-ops, mirroring the middleware
// limiter's arithmetic); HasBudget reports unspent units. The real
// fixed-window semantics — window expiry as the only reset — stay
// pinned in the middleware package's own tests; this fake pins the
// service's check-first/spend-on-failure contract. The production
// wiring (one middleware.NewRateLimiter per surface) is exercised at the
// app and integration seams, which import both packages.
type fakeBudget struct {
	spent   map[string]int
	budget  int
	resetIn time.Duration
}

func newFakeBudget(budget int) *fakeBudget {
	return &fakeBudget{
		spent:   make(map[string]int),
		budget:  budget,
		resetIn: 5 * time.Minute,
	}
}

func (f *fakeBudget) HasBudget(key string) bool {
	return f.spent[key] < f.budget
}

func (f *fakeBudget) Record(key string) {
	if f.spent[key] < f.budget {
		f.spent[key]++
	}
}

func (f *fakeBudget) Limit() int {
	return f.budget
}

func (f *fakeBudget) GetResetIn(key string) time.Duration {
	return f.resetIn
}

// newBudgetTestService seeds a user with fake-brake budget surfaces
// wired the way internal/app wires them — one failurebudget.Surface per
// credential endpoint, here both over the one fake brake (their
// namespaced keys never interact) — so the service tests exercise the
// check/spend contract. The seeded user's ID is "user-" + username.
func newBudgetTestService(t *testing.T, budget int, username, password string) (*AuthService, *testutils.MockSessionRepository, *fakeBudget) {
	t.Helper()

	repo := seedLoginUser(t, username, password)
	sessionRepo := testutils.NewMockSessionRepository()
	b := newFakeBudget(budget)
	service, err := NewTestAuthServiceWithFailureBudget(repo, sessionRepo,
		failurebudget.NewSurface("login", b),
		failurebudget.NewSurface("change-password", b))
	require.NoError(t, err, "Should create auth service with failure budget")
	return service, sessionRepo, b
}

// TestAuthService_Login_FailureBudgetLocksUsernameAfterThreshold pins
// the lockout: after the configured count of wrong-password failures,
// a fresh login for that name answers 429 RATE_LIMIT_EXCEEDED before
// any verification runs — even with the correct password.
func TestAuthService_Login_FailureBudgetLocksUsernameAfterThreshold(t *testing.T) {
	service, _, budget := newBudgetTestService(t, 3, "testuser", "right-password")

	for i := range 3 {
		_, _, _, err := service.Login("testuser", "wrong-password")
		require.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "Failure %d within the budget should stay a 401", i+1)
	}
	require.Equal(t, 3, budget.spent["login:testuser"], "Exactly the three failures should have spent the budget")

	_, _, _, err := service.Login("testuser", "right-password")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded,
		"The login past the threshold should be locked out even with the correct password")
	require.Equal(t, 3, budget.spent["login:testuser"], "A locked-out attempt must not spend further")
}

// TestAuthService_Login_FailureBudgetCountsUnknownUsernames pins that
// an unknown username spends the same budget — the enumeration brake —
// and that one name's spending never touches another name's.
func TestAuthService_Login_FailureBudgetCountsUnknownUsernames(t *testing.T) {
	service, _, budget := newBudgetTestService(t, 1, "realuser", "right-password")

	_, _, _, err := service.Login("ghost", "whatever")
	require.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "The first unknown-username attempt should stay a 401")
	require.Equal(t, 1, budget.spent["login:ghost"], "The unknown username's failure should be counted against it")

	_, _, _, err = service.Login("ghost", "whatever")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded, "The same unknown name should be locked out at once")

	token, _, _, err := service.Login("realuser", "right-password")
	require.NoError(t, err, "Another name's budget must be untouched by ghost's failures")
	require.NotEmpty(t, token, "The real user should still log in")
	require.Equal(t, 0, budget.spent["login:realuser"], "The real user's budget must be untouched")
}

// TestAuthService_Login_SuccessNeverSpendsBudget pins the spend rule:
// only failures consume the budget. Two failures exhaust a budget of
// two — but the successes between them spend nothing, so the
// intermediate attempts stay 401/200 instead of tripping the lockout
// early.
func TestAuthService_Login_SuccessNeverSpendsBudget(t *testing.T) {
	service, _, _ := newBudgetTestService(t, 2, "testuser", "right-password")

	_, _, _, err := service.Login("testuser", "wrong-password")
	require.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "The first failure should stay a 401")

	_, _, _, err = service.Login("testuser", "right-password")
	require.NoError(t, err, "A success must never spend the budget")

	_, _, _, err = service.Login("testuser", "wrong-password")
	require.ErrorIs(t, err, apperrors.ErrInvalidCredentials,
		"The second failure should still be a 401 — the success spent nothing")

	_, _, _, err = service.Login("testuser", "right-password")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded,
		"Exactly the two failures should exhaust the budget, locking even the correct password")
}

// TestAuthService_Login_StorageFailureNeverSpendsBudget pins that a
// repository failure — not a credential verification — leaves the
// budget untouched.
func TestAuthService_Login_StorageFailureNeverSpendsBudget(t *testing.T) {
	repoErr := errors.New("db unavailable")
	repo := testutils.NewMockUserRepositoryWithErrors(nil, nil, repoErr, nil, nil, nil)
	sessionRepo := testutils.NewMockSessionRepository()
	budget := newFakeBudget(1)
	service, err := NewTestAuthServiceWithFailureBudget(repo, sessionRepo, failurebudget.NewSurface("login", budget), nil)
	require.NoError(t, err, "Should create auth service with failure budget")

	_, _, _, err = service.Login("testuser", "right-password")
	require.ErrorIs(t, err, repoErr, "The repository failure should propagate as-is, not as an AppError")
	require.Equal(t, 0, budget.spent["login:testuser"], "A storage failure must not spend the budget")
}

// TestAuthService_Login_LockedUsernameDoesNotTouchRefresh pins the
// lockout's scope: it bars fresh logins, not sessions. A refresh
// session minted before the lockout keeps rotating while its username
// cannot log in at all.
func TestAuthService_Login_LockedUsernameDoesNotTouchRefresh(t *testing.T) {
	service, _, _ := newBudgetTestService(t, 1, "testuser", "right-password")

	_, refreshToken, _, err := service.Login("testuser", "right-password")
	require.NoError(t, err, "The pre-lockout login should mint the refresh session")

	_, _, _, err = service.Login("testuser", "wrong-password")
	require.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "The wrong-password attempt should spend the budget")

	_, _, _, err = service.Login("testuser", "right-password")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded, "Fresh logins should now be locked out")

	accessToken, newRefreshToken, user, err := service.Refresh(refreshToken)
	require.NoError(t, err, "The active refresh session must keep working while the username is login-locked")
	require.NotEmpty(t, accessToken, "Refresh should return a fresh access token")
	require.NotEmpty(t, newRefreshToken, "Refresh should rotate the refresh token")
	require.NotNil(t, user, "Refresh should return the user")
}

// requireInvalidCredentials asserts a wrong-current-password failure:
// WithMessage returns a copy (AppError has no Is), so the sentinel is
// pinned via its code rather than errors.Is.
func requireInvalidCredentials(t *testing.T, err error, msg string) {
	t.Helper()
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok, "%s: failure should be an AppError", msg)
	require.Equal(t, "INVALID_CREDENTIALS", appErr.Code, msg)
}

// TestAuthService_ChangePassword_FailureBudgetCountsWrongCurrentPassword
// pins the caller-keyed budget on the password-change path: wrong
// current-password attempts spend the shared budget (a single wrong
// attempt keeps today's unchanged 401), and once spent, even a correct
// current password answers 429 until the window expires.
func TestAuthService_ChangePassword_FailureBudgetCountsWrongCurrentPassword(t *testing.T) {
	service, sessionRepo, budget := newBudgetTestService(t, 2, "testuser", "old-password")

	// Seed one live session the same way the password tests do.
	_, _, _, err := service.Login("testuser", "old-password")
	require.NoError(t, err, "Should seed a session")

	_, _, _, err = service.ChangePassword("user-testuser", "wrong-current", "new-pass-8")
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok, "Failure should be an AppError")
	assert.Equal(t, "INVALID_CREDENTIALS", appErr.Code, "A single wrong attempt keeps the credentials code")
	assert.Equal(t, "Current password is incorrect", appErr.Message, "A single wrong attempt keeps today's unchanged 401 message")
	require.Equal(t, 1, budget.spent["change-password:user-testuser"], "The wrong current-password attempt should spend the caller's budget")

	_, _, _, err = service.ChangePassword("user-testuser", "wrong-current", "new-pass-8")
	requireInvalidCredentials(t, err, "The second wrong attempt is still within the budget")

	_, _, _, err = service.ChangePassword("user-testuser", "old-password", "new-pass-8")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded,
		"The spent budget must lock the password change even with the correct current password")
	require.Len(t, sessionRepo.SessionsForUser("user-testuser"), 1,
		"The locked-out attempts must have written nothing")
}

// TestAuthService_ChangePassword_SuccessNeverSpendsBudget pins that a
// wrong attempt followed by a successful change leaves the budget able
// to absorb another failure — successes never consume it.
func TestAuthService_ChangePassword_SuccessNeverSpendsBudget(t *testing.T) {
	service, _, budget := newBudgetTestService(t, 2, "testuser", "old-password")

	_, _, _, err := service.ChangePassword("user-testuser", "wrong-current", "new-pass-8")
	requireInvalidCredentials(t, err, "The wrong attempt should stay a 401")

	_, _, _, err = service.ChangePassword("user-testuser", "old-password", "newer-pass-8")
	require.NoError(t, err, "The successful change must go through and spend nothing")
	require.Equal(t, 1, budget.spent["change-password:user-testuser"], "The success must not have spent the budget")

	_, _, _, err = service.ChangePassword("user-testuser", "wrong-current", "new-pass-8")
	// The hash has changed, so the old current password no longer
	// verifies — this is another credential failure, and it fits
	// because the earlier wrong attempt spent exactly one unit.
	requireInvalidCredentials(t, err, "Another failure should still fit in the budget")
	require.Equal(t, 2, budget.spent["change-password:user-testuser"], "Exactly the two failures should have spent the budget")
}
