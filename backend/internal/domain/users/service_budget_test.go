package users

import (
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// fakeBudget is the FailureBudget seam's function-field fake: a
// fixed-count budget per key without the clock. Record spends until the
// budget is gone (further records are no-ops, mirroring the middleware
// limiter's arithmetic); HasBudget reports unspent units. The real
// fixed-window semantics — window expiry as the only reset — stay
// pinned in the middleware package's own tests; this fake pins the
// service's check-first/spend-on-failure contract. The production
// wiring (a dedicated middleware.NewRateLimiter instance for the users
// surface) is exercised at the app and integration seams.
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

// budgetResetFixture is passwordResetFixture with a second admin and a
// fake budget wired, so the reset-specific budget tests can pin whose
// budget a failed verification spends. Both admins share the fixture's
// known password hash.
func budgetResetFixture(t *testing.T, budget int) (*UserService, *testutils.MockUserRepository, *testutils.MockSessionRepository, *fakeBudget) {
	t.Helper()

	service, repo, sessionRepo := passwordResetFixture(t)

	adminHash, err := bcrypt.GenerateFromPassword([]byte("known-old-password"), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash the second admin's password")
	repo.AddUser(&models.User{ID: "admin-2", Username: "root2", Role: models.RoleAdmin, PasswordHash: string(adminHash), CreatedAt: time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)})

	b := newFakeBudget(budget)
	return service.WithFailureBudget(b), repo, sessionRepo, b
}

// TestUserService_ResetPassword_FailureBudgetLocksAdminAfterThreshold
// pins the lockout: after the configured count of wrong-admin-password
// failures, a further reset answers 429 RATE_LIMIT_EXCEEDED before any
// verification runs — even with the correct admin password — and writes
// nothing. The budget is keyed on the acting admin: a second admin
// resetting the same target is unaffected.
func TestUserService_ResetPassword_FailureBudgetLocksAdminAfterThreshold(t *testing.T) {
	service, repo, sessionRepo, budget := budgetResetFixture(t, 2)

	for i := range 2 {
		_, err := service.ResetPassword("admin-1", "user-1", "not-the-admins-password", "brand-new-password")
		require.Error(t, err, "Failure %d within the budget should be attempted", i+1)
		appErr, ok := err.(*apperrors.AppError)
		require.True(t, ok, "Failure %d should be an AppError", i+1)
		require.Equal(t, "INVALID_CREDENTIALS", appErr.Code, "Failure %d within the budget should stay a 401", i+1)
	}
	require.Equal(t, 2, budget.spent[resetBudgetKey("admin-1")], "Exactly the two failures should have spent the acting admin's budget")

	_, err := service.ResetPassword("admin-1", "user-1", "known-old-password", "brand-new-password")
	require.ErrorIs(t, err, apperrors.ErrRateLimitExceeded,
		"The reset past the threshold should be locked out even with the correct admin password")
	require.Equal(t, 2, budget.spent[resetBudgetKey("admin-1")], "A locked-out attempt must not spend further")
	assert.True(t, storedPasswordMatches(t, repo, "user-1", "known-old-password"), "The locked-out attempt must write nothing")
	assert.Len(t, sessionRepo.SessionsForUser("user-1"), 2, "The locked-out attempt must revoke nothing")

	// Keyed on the acting admin: the second admin's budget is untouched,
	// so their reset of the same target goes through.
	_, err = service.ResetPassword("admin-2", "user-1", "known-old-password", "brand-new-password")
	require.NoError(t, err, "Another admin's budget must be untouched by admin-1's failures")
	require.Equal(t, 0, budget.spent[resetBudgetKey("admin-2")], "The second admin must not have spent anything")
}

// TestUserService_ResetPassword_UnknownTargetAnswers404WithSpentBudget
// pins the validation order (issue 196 review): the target lookup runs
// before the budget check, so an unknown target answers ErrUserNotFound
// — 404, unchanged — even for a locked-out caller, and the lookup
// verifies nothing, so it spends nothing either.
func TestUserService_ResetPassword_UnknownTargetAnswers404WithSpentBudget(t *testing.T) {
	service, _, _, budget := budgetResetFixture(t, 1)

	_, err := service.ResetPassword("admin-1", "user-1", "not-the-admins-password", "brand-new-password")
	require.Error(t, err, "The wrong-password failure should spend the budget")

	_, err = service.ResetPassword("admin-1", "no-such-user", "not-the-admins-password", "brand-new-password")
	require.ErrorIs(t, err, apperrors.ErrUserNotFound,
		"The spent budget must not turn an unknown target into a 429 — unknown targets stay 404")
	require.Equal(t, 1, budget.spent[resetBudgetKey("admin-1")],
		"The unknown-target lookup must not spend further — it verifies nothing")
}

// TestUserService_ResetPassword_SuccessNeverSpendsBudget pins the spend
// rule: only failed verifications consume the budget. A failure, a
// successful reset, and another failure spend exactly two units — the
// success between them spends nothing — so the second failure still
// fits before the lockout bites.
func TestUserService_ResetPassword_SuccessNeverSpendsBudget(t *testing.T) {
	service, _, _, budget := budgetResetFixture(t, 2)

	_, err := service.ResetPassword("admin-1", "user-1", "not-the-admins-password", "brand-new-password")
	require.Error(t, err, "The first failure should be attempted")

	_, err = service.ResetPassword("admin-1", "user-2", "known-old-password", "brand-new-password")
	require.NoError(t, err, "A successful reset must go through and spend nothing")
	require.Equal(t, 1, budget.spent[resetBudgetKey("admin-1")], "The success must not have spent the budget")

	_, err = service.ResetPassword("admin-1", "user-1", "not-the-admins-password", "brand-new-password")
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok, "The second failure should be an AppError")
	require.Equal(t, "INVALID_CREDENTIALS", appErr.Code,
		"The second failure should still be attempted — the success spent nothing")
	require.Equal(t, 2, budget.spent[resetBudgetKey("admin-1")], "Exactly the two failures should have spent the budget")
}

// TestUserService_ResetPassword_SelfGuardNeverSpendsBudget pins the
// guard ordering: a self reset is refused before the budget is even
// consulted, so a doomed request burns no bcrypt compare and no budget
// — a later wrong-password attempt on another target still fits.
func TestUserService_ResetPassword_SelfGuardNeverSpendsBudget(t *testing.T) {
	service, _, _, budget := budgetResetFixture(t, 1)

	_, err := service.ResetPassword("admin-1", "admin-1", "not-the-admins-password", "brand-new-password")
	require.ErrorIs(t, err, apperrors.ErrSelfPasswordReset, "The self reset must surface SELF_PASSWORD_RESET")
	require.Equal(t, 0, budget.spent[resetBudgetKey("admin-1")], "A refused self reset must not spend the budget")

	_, err = service.ResetPassword("admin-1", "user-1", "not-the-admins-password", "brand-new-password")
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok, "The wrong-password attempt should be an AppError")
	require.Equal(t, "INVALID_CREDENTIALS", appErr.Code,
		"The wrong-password attempt should still be attempted — the self reset spent nothing")
	require.Equal(t, 1, budget.spent[resetBudgetKey("admin-1")], "The wrong-password failure should spend the budget")
}
