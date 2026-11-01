package auth

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// medianFailedLoginLatency times runs failed Login calls and returns the
// median latency. The median (not the mean) keeps one scheduler hiccup
// from skewing the measurement.
func medianFailedLoginLatency(t *testing.T, runs int, login func() error) time.Duration {
	t.Helper()

	latencies := make([]time.Duration, 0, runs)
	for range runs {
		start := time.Now()
		require.Error(t, login(), "Every measured login must fail")
		latencies = append(latencies, time.Since(start))
	}
	slices.Sort(latencies)
	return latencies[runs/2]
}

// TestAuthService_Login_TimingEqualization pins the timing equalization
// that closes the username-enumeration oracle of pentest 2026-11-01
// finding 2: an unknown-username login must pay a bcrypt compare
// against the fixed dummy hash, just as a wrong-password login pays the
// real compare, so the two 401 AUTH_INVALID_CREDENTIALS answers are
// latency-indistinguishable. The bound is deliberately generous — the
// unknown-username median must reach at least half the wrong-password
// median over several runs — because it exists to catch the oracle's
// absence (a ratio near zero without the dummy compare), not to measure
// bcrypt's cost precisely.
func TestAuthService_Login_TimingEqualization(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	const runs = 7

	unknownUsernameMedian := medianFailedLoginLatency(t, runs, func() error {
		_, _, _, _, err := service.Login("nonexistent-user", "some-password", false)
		return err
	})
	wrongPasswordMedian := medianFailedLoginLatency(t, runs, func() error {
		_, _, _, _, err := service.Login("testuser", "wrong-password-456", false)
		return err
	})

	require.GreaterOrEqual(t,
		unknownUsernameMedian, wrongPasswordMedian/2,
		"Unknown-username login must cost at least half of a wrong-password login (got %s vs %s); without the dummy bcrypt compare the unknown-username path answers near-instantly and leaks which usernames exist",
		unknownUsernameMedian, wrongPasswordMedian,
	)
}

// TestAuthService_Login_DummyHashCost pins the dummy hash's contract:
// a package-level const whose cost is exactly bcrypt.DefaultCost, the
// same cost as the real compare, so the equalization of the timing test
// above is structural and not an accident of a cheaper hash.
func TestAuthService_Login_DummyHashCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyLoginHash))
	require.NoError(t, err, "The dummy login hash must be a well-formed bcrypt hash")
	require.Equal(t, bcrypt.DefaultCost, cost, "The dummy login hash must sit at bcrypt.DefaultCost, the same cost as the real compare")
}
