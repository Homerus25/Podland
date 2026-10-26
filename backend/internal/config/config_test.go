package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_SessionTTLDefaultsTo30Days asserts the refresh-session TTL
// defaults to 30 days when SESSION_TTL_DAYS is unset (parent PRD:
// sessions stay valid across browser restarts for a month by default).
func TestLoad_SessionTTLDefaultsTo30Days(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("SESSION_TTL_DAYS", "")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with no session TTL set")

	assert.Equal(t, 30, cfg.Session.TTLDays, "Session TTL should default to 30 days")
}

// TestLoad_SessionTTLOverride asserts SESSION_TTL_DAYS is honoured when set.
func TestLoad_SessionTTLOverride(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("SESSION_TTL_DAYS", "7")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with SESSION_TTL_DAYS=7")

	assert.Equal(t, 7, cfg.Session.TTLDays, "Session TTL should come from the environment")
}

// TestLoad_AuthRateLimitDefaultsTo10PerIPPerMinute asserts the auth
// limiter (login/refresh, keyed by client IP) defaults to a
// brute-force-hostile 10 requests per minute when its env var is unset.
func TestLoad_AuthRateLimitDefaultsTo10PerIPPerMinute(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE", "")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with no auth rate limit set")

	assert.Equal(t, 10, cfg.RateLimit.AuthRequestsPerMinute, "Auth rate limit should default to 10 per IP per minute")
}

// TestLoad_AuthRateLimitOverride asserts RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE
// is honoured when set.
func TestLoad_AuthRateLimitOverride(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE", "30")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with the auth rate limit override")

	assert.Equal(t, 30, cfg.RateLimit.AuthRequestsPerMinute, "Auth rate limit should come from the environment")
}

// TestLoad_AuthFailureBudgetDefaults pins the credential-failure
// budget's defaults when neither env var is set: five failures per
// fifteen-minute window (issue 186).
func TestLoad_AuthFailureBudgetDefaults(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("RATE_LIMIT_AUTH_FAILURE_BUDGET", "")
	t.Setenv("RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES", "")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with no failure-budget env set")

	assert.Equal(t, 5, cfg.RateLimit.FailureBudget, "Failure budget should default to 5")
	assert.Equal(t, 15, cfg.RateLimit.FailureWindowMinutes, "Failure window should default to 15 minutes")
}

// TestLoad_AuthFailureBudgetOverride asserts the shared budget pair is
// environment-configurable as one pair.
func TestLoad_AuthFailureBudgetOverride(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("RATE_LIMIT_AUTH_FAILURE_BUDGET", "3")
	t.Setenv("RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES", "30")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with the failure-budget override")

	assert.Equal(t, 3, cfg.RateLimit.FailureBudget, "Failure budget should come from the environment")
	assert.Equal(t, 30, cfg.RateLimit.FailureWindowMinutes, "Failure window should come from the environment")
}

// TestValidate_RejectsNonPositiveSessionTTL asserts a zero or negative
// TTL is rejected at startup rather than expiring sessions instantly.
func TestValidate_RejectsNonPositiveSessionTTL(t *testing.T) {
	for _, days := range []int{0, -1} {
		cfg := &Config{
			Encryption: EncryptionConfig{Key: "k"},
			Session:    SessionConfig{TTLDays: days},
		}

		err := cfg.Validate()

		assert.Error(t, err, "TTL of %d day(s) should fail validation", days)
	}
}

// TestValidate_RejectsNonPositiveFailureBudget asserts a zero or
// negative failure budget or window is rejected at startup — either
// zero would silently lock every fresh login out of the instance.
func TestValidate_RejectsNonPositiveFailureBudget(t *testing.T) {
	for _, tc := range []struct{ budget, window int }{
		{0, 15}, {-1, 15},
		{5, 0}, {5, -1},
	} {
		cfg := &Config{
			Encryption: EncryptionConfig{Key: "k"},
			Session:    SessionConfig{TTLDays: 30},
			RateLimit:  RateLimitConfig{FailureBudget: tc.budget, FailureWindowMinutes: tc.window},
		}

		err := cfg.Validate()

		assert.Error(t, err, "Budget %d / window %d should fail validation", tc.budget, tc.window)
	}
}

// TestValidate_RejectsInvalidSessionTTLEnvValue asserts a non-numeric
// SESSION_TTL_DAYS falls back to the default instead of yielding a
// zero TTL (matching getEnvInt semantics for every other setting).
func TestValidate_RejectsInvalidSessionTTLEnvValue(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("SESSION_TTL_DAYS", "not-a-number")

	cfg, err := Load()
	require.NoError(t, err, "Load should fall back to the default for a non-numeric value")

	assert.Equal(t, 30, cfg.Session.TTLDays, "Non-numeric SESSION_TTL_DAYS should fall back to 30")
	require.NoError(t, cfg.Validate(), "Fallback config should validate")
}

// TestLoad_SessionRememberTTLDefaultsTo90Days asserts the remembered
// session TTL defaults to 90 days when SESSION_REMEMBER_TTL_DAYS is
// unset (issue 379: a remembered device stays signed in for a quarter
// by default, three times the unremembered window).
func TestLoad_SessionRememberTTLDefaultsTo90Days(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("SESSION_REMEMBER_TTL_DAYS", "")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with no remembered TTL set")

	assert.Equal(t, 90, cfg.Session.RememberTTLDays, "Remembered session TTL should default to 90 days")
}

// TestLoad_SessionRememberTTLOverride asserts SESSION_REMEMBER_TTL_DAYS
// is honoured when set.
func TestLoad_SessionRememberTTLOverride(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key")
	t.Setenv("SESSION_REMEMBER_TTL_DAYS", "180")

	cfg, err := Load()
	require.NoError(t, err, "Load should succeed with SESSION_REMEMBER_TTL_DAYS=180")

	assert.Equal(t, 180, cfg.Session.RememberTTLDays, "Remembered session TTL should come from the environment")
}

// TestValidate_RejectsNonPositiveRememberTTL asserts a zero or negative
// remembered TTL is rejected at startup — it would expire remembered
// sessions instantly (issue 379). The base config is fully valid, so
// the failure can only come from the remember TTL under test.
func TestValidate_RejectsNonPositiveRememberTTL(t *testing.T) {
	for _, days := range []int{0, -1} {
		cfg := &Config{
			Encryption: EncryptionConfig{Key: "k"},
			Session:    SessionConfig{TTLDays: 30, RememberTTLDays: 90},
			RateLimit:  RateLimitConfig{FailureBudget: 5, FailureWindowMinutes: 15},
		}
		require.NoError(t, cfg.Validate(), "Base config should validate")

		cfg.Session.RememberTTLDays = days

		err := cfg.Validate()

		assert.Error(t, err, "Remembered TTL of %d day(s) should fail validation", days)
	}
}

// TestValidate_RejectsRememberTTLBelowSessionTTL asserts a remembered
// TTL shorter than the unremembered one is rejected at startup: a
// remembered session that expires sooner than a plain one is a
// configuration bug (issue 379). Equality is legal — tightening both
// knobs to the same value must not brick startup.
func TestValidate_RejectsRememberTTLBelowSessionTTL(t *testing.T) {
	cfg := &Config{
		Encryption: EncryptionConfig{Key: "k"},
		Session:    SessionConfig{TTLDays: 30, RememberTTLDays: 29},
		RateLimit:  RateLimitConfig{FailureBudget: 5, FailureWindowMinutes: 15},
	}

	err := cfg.Validate()

	assert.Error(t, err, "Remembered TTL below SESSION_TTL_DAYS should fail validation")

	cfg.Session.RememberTTLDays = 30
	assert.NoError(t, cfg.Validate(), "Remembered TTL equal to SESSION_TTL_DAYS should validate")
}

// TestLoad_VolumeUploadMaxBytes pins the populate upload cap: default
// 2 GiB, honored override, and fallback to the default on garbage.
func TestLoad_VolumeUploadMaxBytes(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, int64(2<<30), cfg.Volumes.MaxUploadBytes, "default cap must be 2 GiB")

	t.Setenv("VOLUME_UPLOAD_MAX_BYTES", "1024")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, int64(1024), cfg.Volumes.MaxUploadBytes, "env override must be honored")

	t.Setenv("VOLUME_UPLOAD_MAX_BYTES", "not-a-number")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, int64(2<<30), cfg.Volumes.MaxUploadBytes, "garbage values must fall back to the default")
}

// TestLoad_ImageTransferMaxBytes pins the cross-machine image copy's
// transfer bound: default 10 GiB (the kit's DefaultMaxImageTransferBytes
// — images are routinely larger than the volume transfer's 2 GiB),
// honored override, and fallback to the default on garbage.
func TestLoad_ImageTransferMaxBytes(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, int64(10<<30), cfg.Images.MaxTransferBytes, "default bound must be 10 GiB")

	t.Setenv("IMAGE_TRANSFER_MAX_BYTES", "1024")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, int64(1024), cfg.Images.MaxTransferBytes, "env override must be honored")

	t.Setenv("IMAGE_TRANSFER_MAX_BYTES", "not-a-number")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, int64(10<<30), cfg.Images.MaxTransferBytes, "garbage values must fall back to the default")
}
