package failurebudget

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingBrake is the Brake seam's function-field fake: it records
// every key it is asked about so the tests can pin the key derivation,
// and answers HasBudget from a spent-set the tests control.
type recordingBrake struct {
	spent   map[string]int
	budget  int
	resetIn time.Duration
}

func newRecordingBrake(budget int) *recordingBrake {
	return &recordingBrake{spent: make(map[string]int), budget: budget, resetIn: 5 * time.Minute}
}

func (b *recordingBrake) HasBudget(key string) bool {
	return b.spent[key] < b.budget
}

func (b *recordingBrake) Record(key string) {
	b.spent[key]++
}

func (b *recordingBrake) Limit() int {
	return b.budget
}

func (b *recordingBrake) GetResetIn(key string) time.Duration {
	return b.resetIn
}

// TestSurface_NilIsTheUnwiredState pins the nil-receiver contract all
// three methods carry: a nil *Surface leaves the brake unwired — Check
// always reports budget, Spend records nothing, and Advertise writes
// nothing — without panicking. This is the invariant the domains'
// former nil-guard pair collapsed into.
func TestSurface_NilIsTheUnwiredState(t *testing.T) {
	var surface *Surface
	brake := newRecordingBrake(0)

	assert.True(t, surface.Check("anyone"), "An unwired surface must always report budget")

	surface.Spend("anyone")
	assert.Empty(t, brake.spent, "An unwired surface must record nothing")

	w := httptest.NewRecorder()
	surface.Advertise(w, "anyone", apperrors.ErrRateLimitExceeded)
	assert.Empty(t, w.Header(), "An unwired surface must advertise nothing")
}

// TestSurface_NamespacingDisjoinsKeys pins the issue-196 invariant,
// made structural: two Surfaces over one Brake derive disjoint keys for
// the same id, so one endpoint's spending never locks the other — even
// when the id is identical (a username and a user ID cannot collide).
func TestSurface_NamespacingDisjoinsKeys(t *testing.T) {
	brake := newRecordingBrake(1)
	login := NewSurface("login", brake)
	change := NewSurface("change-password", brake)

	require.True(t, login.Check("same-id"), "The login surface's first check should report budget")
	login.Spend("same-id")
	assert.Equal(t, 1, brake.spent["login:same-id"], "The login surface should spend under its own namespaced key")

	assert.False(t, login.Check("same-id"), "The login surface's budget for the id should be spent")
	assert.True(t, change.Check("same-id"),
		"The change surface must be unaffected — the same id under another surface is another key")
	assert.Equal(t, 0, brake.spent["change-password:same-id"], "The change surface's key must be untouched")
}

// TestSurface_CheckAndSpendRideTheBrake pins the check/spend pair's
// pass-through: Check reads the brake without spending, Spend records
// exactly the id's key.
func TestSurface_CheckAndSpendRideTheBrake(t *testing.T) {
	brake := newRecordingBrake(2)
	surface := NewSurface("reset-password", brake)

	assert.True(t, surface.Check("admin-1"), "An unspent budget should check true")
	assert.Equal(t, 0, brake.spent["reset-password:admin-1"], "Check must be a pure read")

	surface.Spend("admin-1")
	surface.Spend("admin-1")
	assert.Equal(t, 2, brake.spent["reset-password:admin-1"], "Spend should record the namespaced key once per call")
	assert.False(t, surface.Check("admin-1"), "A spent budget should check false")
}

// TestSurface_AdvertiseWritesTrioOnBudget429 pins the advertisement's
// positive case: the budget's own 429 — an AppError with status 429 —
// carries the full X-RateLimit trio, with the brake's configured limit,
// zero remaining, and the brake's reset window.
func TestSurface_AdvertiseWritesTrioOnBudget429(t *testing.T) {
	brake := newRecordingBrake(5)
	brake.resetIn = 90 * time.Second
	surface := NewSurface("login", brake)

	w := httptest.NewRecorder()
	surface.Advertise(w, "testuser", apperrors.ErrRateLimitExceeded)
	assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"), "The 429 should advertise the brake's configured limit")
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"), "The 429 should report no remaining budget")
	assert.Equal(t, "90", w.Header().Get("X-RateLimit-Reset"), "The 429 should report the brake's reset window in seconds")
}

// TestSurface_AdvertiseIgnoresOtherErrors pins the classification: only
// the budget's own 429 AppError is advertised. A non-429 AppError, a
// non-AppError, and a nil error all write nothing — the trio is the
// budget's verdict, not domain error mapping.
func TestSurface_AdvertiseIgnoresOtherErrors(t *testing.T) {
	surface := NewSurface("login", newRecordingBrake(5))

	cases := []struct {
		name string
		err  error
	}{
		{"non-429 AppError", apperrors.ErrInvalidCredentials},
		{"non-AppError", errors.New("db unavailable")},
		{"nil error", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			surface.Advertise(w, "testuser", tc.err)
			assert.Empty(t, w.Header(), "A %s must not carry rate-limit headers", tc.name)
		})
	}
}
