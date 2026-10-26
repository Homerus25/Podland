package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"log/slog"

	"github.com/Homerus25/podland/backend/internal/transfer"
)

// Config holds all application configuration
type Config struct {
	Port           int
	Database       DatabaseConfig
	JWT            JWTConfig
	Session        SessionConfig
	Connections    ConnectionConfig
	RateLimit      RateLimitConfig
	AllowedOrigins []string
	Log            LogConfig
	Encryption     EncryptionConfig
	Volumes        VolumeConfig
}

// VolumeConfig holds volume configuration
type VolumeConfig struct {
	// MaxUploadBytes bounds populate uploads; parts are streamed to
	// disk and the cap is enforced while streaming (413 past it).
	MaxUploadBytes int64
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	PrivateKeyPath    string
	PublicKeyPath     string
	ExpirationMinutes int
	Issuer            string
}

// SessionConfig holds refresh-session configuration. The TTL is the
// window a refresh cookie stays redeemable; a new session is created
// at every login with this expiry. RememberTTLDays is the window a
// login that opted in via the request's rememberDevice field stays
// redeemable (issue 379) — it must be at least TTLDays, so a
// remembered session can never expire sooner than a plain one.
type SessionConfig struct {
	TTLDays         int
	RememberTTLDays int
}

// ConnectionConfig holds connection configuration
type ConnectionConfig struct {
	SSHIdleTimeoutSeconds int
	MaxConcurrent         int
	TestTimeoutSeconds    int
}

// RateLimitConfig holds rate limiting configuration. RequestsPerMinute
// budgets the authenticated API surface per user; AuthRequestsPerMinute
// budgets the unauthenticated auth surface (login, refresh) per client
// IP — those routes have no user to key on. FailureBudget and
// FailureWindowMinutes are the credential-failure budget pair behind
// every password-verification surface: the auth surface's limiter
// instance covers login (keyed per username) and self-service password
// change (keyed per caller), the users surface's admin password reset
// holds its own instance over the same pair (keyed per acting admin) —
// one limiter instance per surface, so the key spaces stay separate.
// The env names keep their historical AUTH segment.
type RateLimitConfig struct {
	RequestsPerMinute     int
	AuthRequestsPerMinute int
	FailureBudget         int
	FailureWindowMinutes  int
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	Key string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	// Server port
	if port := os.Getenv("PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Port = p
		} else {
			cfg.Port = 8080
		}
	} else {
		cfg.Port = 8080
	}

	// Database
	cfg.Database.Path = getEnv("DATABASE_PATH", "./podland.db")

	// JWT
	cfg.JWT.PrivateKeyPath = getEnv("JWT_PRIVATE_KEY_PATH", "./jwt-private.pem")
	cfg.JWT.PublicKeyPath = getEnv("JWT_PUBLIC_KEY_PATH", "./jwt-public.pem")
	cfg.JWT.ExpirationMinutes = getEnvInt("JWT_EXPIRATION_MINUTES", 60)
	cfg.JWT.Issuer = getEnv("JWT_ISSUER", "podland")

	// Sessions
	cfg.Session.TTLDays = getEnvInt("SESSION_TTL_DAYS", 30)
	cfg.Session.RememberTTLDays = getEnvInt("SESSION_REMEMBER_TTL_DAYS", 90)

	// Connections
	cfg.Connections.SSHIdleTimeoutSeconds = getEnvInt("SSH_IDLE_TIMEOUT_SECONDS", 900)
	cfg.Connections.MaxConcurrent = getEnvInt("MAX_CONCURRENT_MACHINES", 100)
	cfg.Connections.TestTimeoutSeconds = getEnvInt("MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS", 10)

	// Rate Limiting
	cfg.RateLimit.RequestsPerMinute = getEnvInt("RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE", 60)
	// Brute-force-hostile default: login+refresh share one per-IP bucket.
	cfg.RateLimit.AuthRequestsPerMinute = getEnvInt("RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE", 10)
	// Credential-failure budget: five failures per fifteen minutes is
	// the classic brake — enough for typos, hostile to guessing.
	cfg.RateLimit.FailureBudget = getEnvInt("RATE_LIMIT_AUTH_FAILURE_BUDGET", 5)
	cfg.RateLimit.FailureWindowMinutes = getEnvInt("RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES", 15)

	// CORS
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		cfg.AllowedOrigins = strings.Split(origins, ",")
	}

	// Logging
	cfg.Log.Level = getEnv("LOG_LEVEL", "info")
	cfg.Log.Format = getEnv("LOG_FORMAT", "json")

	// Encryption
	cfg.Encryption.Key = getEnv("ENCRYPTION_KEY", "")

	// Volumes
	// The default is the transfer kit's DefaultMaxVolumeUploadBytes —
	// the same bound the cross-machine copy/move falls back to — so
	// the uploads cap and the transfer bound share one 2 GiB policy.
	cfg.Volumes.MaxUploadBytes = getEnvInt64("VOLUME_UPLOAD_MAX_BYTES", transfer.DefaultMaxVolumeUploadBytes)

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt returns the value of an environment variable as int or a default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
	}
	return defaultValue
}

// getEnvInt64 returns the value of an environment variable as int64 or
// a default
func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			return v
		}
	}
	return defaultValue
}

// NewLogger creates a new slog.Logger based on configuration
func NewLogger(config LogConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(config.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if strings.EqualFold(config.Format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.Encryption.Key == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required")
	}
	if cfg.Session.TTLDays <= 0 {
		return fmt.Errorf("SESSION_TTL_DAYS must be a positive number of days")
	}
	if cfg.Session.RememberTTLDays <= 0 {
		return fmt.Errorf("SESSION_REMEMBER_TTL_DAYS must be a positive number of days")
	}
	if cfg.Session.RememberTTLDays < cfg.Session.TTLDays {
		return fmt.Errorf("SESSION_REMEMBER_TTL_DAYS must be greater than or equal to SESSION_TTL_DAYS")
	}
	if cfg.RateLimit.FailureBudget <= 0 {
		return fmt.Errorf("RATE_LIMIT_AUTH_FAILURE_BUDGET must be a positive number of failures")
	}
	if cfg.RateLimit.FailureWindowMinutes <= 0 {
		return fmt.Errorf("RATE_LIMIT_AUTH_FAILURE_WINDOW_MINUTES must be a positive number of minutes")
	}
	return nil
}
