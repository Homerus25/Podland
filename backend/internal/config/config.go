package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
// at every login with this expiry.
type SessionConfig struct {
	TTLDays int
}

// ConnectionConfig holds connection configuration
type ConnectionConfig struct {
	SSHIdleTimeoutSeconds int
	MaxConcurrent         int
	QueueTimeoutSeconds   int
	TestTimeoutSeconds    int
}

// RateLimitConfig holds rate limiting configuration. RequestsPerMinute
// budgets the authenticated API surface per user; AuthRequestsPerMinute
// budgets the unauthenticated auth surface (login, refresh) per client
// IP — those routes have no user to key on.
type RateLimitConfig struct {
	RequestsPerMinute     int
	AuthRequestsPerMinute int
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

	// Connections
	cfg.Connections.SSHIdleTimeoutSeconds = getEnvInt("SSH_IDLE_TIMEOUT_SECONDS", 900)
	cfg.Connections.MaxConcurrent = getEnvInt("MAX_CONCURRENT_MACHINES", 100)
	cfg.Connections.QueueTimeoutSeconds = getEnvInt("CONNECTION_QUEUE_TIMEOUT_SECONDS", 30)
	cfg.Connections.TestTimeoutSeconds = getEnvInt("MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS", 10)

	// Rate Limiting
	cfg.RateLimit.RequestsPerMinute = getEnvInt("RATE_LIMIT_REQUESTS_PER_USER_PER_MINUTE", 60)
	// Brute-force-hostile default: login+refresh share one per-IP bucket.
	cfg.RateLimit.AuthRequestsPerMinute = getEnvInt("RATE_LIMIT_AUTH_REQUESTS_PER_IP_PER_MINUTE", 10)

	// CORS
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		cfg.AllowedOrigins = strings.Split(origins, ",")
	}

	// Logging
	cfg.Log.Level = getEnv("LOG_LEVEL", "info")
	cfg.Log.Format = getEnv("LOG_FORMAT", "json")

	// Encryption
	cfg.Encryption.Key = getEnv("ENCRYPTION_KEY", "")

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

// NewLogger creates a new logger based on configuration
// TODO: Implement proper logging when logrus is added
func NewLogger(config LogConfig) any {
	// Placeholder for logger
	return nil
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.Encryption.Key == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required")
	}
	if cfg.Session.TTLDays <= 0 {
		return fmt.Errorf("SESSION_TTL_DAYS must be a positive number of days")
	}
	return nil
}
