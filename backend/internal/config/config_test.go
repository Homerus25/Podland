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
