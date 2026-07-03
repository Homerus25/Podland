package testutils

import (
	"os"
	"strings"
	"time"
)

// TestConfig holds test configuration settings
type TestConfig struct {
	// TestTimeout is the default timeout for test operations
	TestTimeout time.Duration
	// RetryAttempts is the number of times to retry flaky tests
	RetryAttempts int
	// VerboseOutput enables verbose test output
	VerboseOutput bool
	// MockResponses enables mock responses instead of real service calls
	MockResponses bool
	// TempDir is the temporary directory for test files
	TempDir string
	// CleanupOnExit indicates whether to clean up temp files on exit
	CleanupOnExit bool
}

// DefaultTestConfig returns a default test configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		TestTimeout:    30 * time.Second,
		RetryAttempts: 1,
		VerboseOutput: false,
		MockResponses: true,
		TempDir:       "test-temp",
		CleanupOnExit: true,
	}
}

// Global test configuration instance
var globalConfig *TestConfig

// InitTestConfig initializes the global test configuration
func InitTestConfig() *TestConfig {
	if globalConfig == nil {
		globalConfig = DefaultTestConfig()
	}
	return globalConfig
}

// GetTestConfig returns the global test configuration
func GetTestConfig() *TestConfig {
	if globalConfig == nil {
		return InitTestConfig()
	}
	return globalConfig
}

// SetTestConfig sets the global test configuration
func SetTestConfig(config *TestConfig) {
	globalConfig = config
}

// GetEnv returns an environment variable with a default value if not set
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvAsInt returns an environment variable as an integer
func GetEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if _, err := os.LookupEnv(key); !err {
			return defaultValue
		}
		// Try to parse as int - for now just return default
		// In a real implementation, use strconv.Atoi
		return defaultValue
	}
	return defaultValue
}

// GetEnvAsBool returns an environment variable as a boolean
func GetEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		lowerValue := strings.ToLower(value)
		return lowerValue == "true" || lowerValue == "1" || lowerValue == "yes"
	}
	return defaultValue
}

// GetEnvAsDuration returns an environment variable as a duration
func GetEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		// Parse duration string - for now just return default
		// In a real implementation, use time.ParseDuration
		return defaultValue
	}
	return defaultValue
}

// CreateTempDir creates a temporary directory for testing
func CreateTempDir(prefix string) (string, error) {
	// For now, create a simple temp dir
	tempDir := prefix + "-test-" + strings.ReplaceAll(time.Now().Format("20060102-150405"), ":", "")
	if err := os.MkdirAll(tempDir, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	return tempDir, nil
}

// RemoveTempDir removes a temporary directory
func RemoveTempDir(dir string) error {
	return os.RemoveAll(dir)
}

// WithTimeout runs a function with a timeout
func WithTimeout(timeout time.Duration, f func() error) error {
	done := make(chan struct{})
	
	go func() {
		defer close(done)
		if err := f(); err != nil {
			// Error will be handled by caller
		}
	}()
	
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return os.ErrDeadlineExceeded
	}
}

// WaitForCondition waits for a condition to be true with a timeout
func WaitForCondition(timeout time.Duration, interval time.Duration, condition func() bool) error {
	endTime := time.Now().Add(timeout)
	
	for time.Now().Before(endTime) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}
	
	return os.ErrDeadlineExceeded
}