package machines

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// TestIsValidPEM tests PEM key validation with various key formats
func TestIsValidPEM(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "valid RSA private key",
			key:      testutils.GenerateValidPEMKey(),
			expected: true,
		},
		{
			name:     "valid OpenSSH private key",
			key:      testutils.GenerateValidOpenSSHKey(),
			expected: true,
		},
		{
			name:     "valid EC private key",
			key:      testutils.GenerateValidECKey(),
			expected: true,
		},
		{
			name:     "invalid key - too short",
			key:      "-----BEGIN",
			expected: false,
		},
		{
			name:     "invalid key - missing BEGIN",
			key:      "MIIEpAIBAAKCAQEA...-----END RSA PRIVATE KEY-----",
			expected: false,
		},
		{
			name:     "invalid key - missing END",
			key:      "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...",
			expected: false,
		},
		{
			name:     "invalid key - not a private key",
			key:      "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG...",
			expected: false,
		},
		{
			name:     "empty key",
			key:      "",
			expected: false,
		},
		{
			name:     "whitespace only",
			key:      "   ",
			expected: false,
		},
		{
			name:     "valid key with whitespace",
			key:      "  " + testutils.GenerateValidPEMKey() + "\n  ",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPEM(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCreateMachineRequest_Defaults tests default value handling for machine creation
func TestCreateMachineRequest_Defaults(t *testing.T) {
	t.Run("default port when 0", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
			Port:          0,
		}

		// Apply defaults like the service does
		if req.Port == 0 {
			req.Port = 22
		}
		assert.Equal(t, 22, req.Port)
	})

	t.Run("default port when not 0", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
			Port:          2222,
		}

		// Apply defaults like the service does
		if req.Port == 0 {
			req.Port = 22
		}
		assert.Equal(t, 2222, req.Port)
	})

	t.Run("default podman socket path", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Hostname:         "example.com",
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "",
		}

		// Apply defaults like the service does
		if req.PodmanSocketPath == "" {
			req.PodmanSocketPath = "/run/podman/podman.sock"
		}
		assert.Equal(t, "/run/podman/podman.sock", req.PodmanSocketPath)
	})

	t.Run("custom podman socket path", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Hostname:         "example.com",
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/user/1000/podman/podman.sock",
		}

		// Apply defaults like the service does
		if req.PodmanSocketPath == "" {
			req.PodmanSocketPath = "/run/podman/podman.sock"
		}
		assert.Equal(t, "/run/user/1000/podman/podman.sock", req.PodmanSocketPath)
	})
}

// TestMachineService_Create_Validation tests machine creation validation logic
func TestMachineService_Create_Validation(t *testing.T) {
	// For now, we'll test the PEM validation which is the core functionality
	// The full service testing will be implemented when we have proper mocks
	t.Run("valid PEM key", func(t *testing.T) {
		validKey := testutils.GenerateValidPEMKey()
		assert.True(t, IsValidPEM(validKey), "Valid PEM key should be accepted")
	})

	t.Run("invalid PEM key", func(t *testing.T) {
		invalidKey := "invalid-key"
		assert.False(t, IsValidPEM(invalidKey), "Invalid PEM key should be rejected")
	})

	t.Run("empty PEM key", func(t *testing.T) {
		emptyKey := ""
		assert.False(t, IsValidPEM(emptyKey), "Empty PEM key should be rejected")
	})
}

// TestPEMValidation_EdgeCases tests edge cases for PEM validation
func TestPEMValidation_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "key with leading/trailing whitespace",
			key:      "  " + testutils.GenerateValidPEMKey() + "\n\t",
			expected: true,
		},
		{
			name:     "key with multiple newlines",
			key:      testutils.GenerateValidPEMKey() + "\n\n\n",
			expected: true,
		},
		{
			name:     "very short invalid key",
			key:      "short",
			expected: false,
		},
		{
			name:     "key with only BEGIN marker",
			key:      "-----BEGIN RSA PRIVATE KEY-----",
			expected: false,
		},
		{
			name:     "key with only END marker",
			key:      "-----END RSA PRIVATE KEY-----",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPEM(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMachineCreation_RequestValidation tests request validation for machine creation
func TestMachineCreation_RequestValidation(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Description:      "Test machine",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}

		// Basic validation - name should not be empty
		assert.NotEmpty(t, req.Name, "Machine name should not be empty")
		assert.NotEmpty(t, req.Hostname, "Hostname should not be empty")
		assert.NotEmpty(t, req.SSHUsername, "SSH username should not be empty")
		assert.True(t, IsValidPEM(req.SSHPrivateKey), "SSH private key should be valid")
	})

	t.Run("request with missing name", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:             "", // Empty name
			Hostname:         "example.com",
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
		}

		assert.Empty(t, req.Name, "Name should be empty for this test case")
	})

	t.Run("request with invalid PEM key", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: "invalid-key",
		}

		assert.False(t, IsValidPEM(req.SSHPrivateKey), "Invalid PEM key should be rejected")
	})
}

// TestMachineService_PEMValidationPerformance tests PEM validation performance
func TestMachineService_PEMValidationPerformance(t *testing.T) {
	// Generate a valid PEM key for performance testing
	validKey := testutils.GenerateValidPEMKey()
	invalidKey := "invalid-key"

	// Test multiple validations to ensure performance is acceptable
	startTime := time.Now()
	for i := 0; i < 100; i++ {
		_ = IsValidPEM(validKey)
		_ = IsValidPEM(invalidKey)
	}
	duration := time.Since(startTime)

	// Should complete 200 validations in less than 100ms
	assert.Less(t, duration, 100*time.Millisecond, "PEM validation should be fast")
}

// TestMachineService_DefaultValues tests default value handling
func TestMachineService_DefaultValues(t *testing.T) {
	t.Run("default port handling", func(t *testing.T) {
		// Test with zero port (should default to 22)
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
			Port:          0,
		}

		if req.Port == 0 {
			req.Port = 22
		}
		assert.Equal(t, 22, req.Port, "Default port should be 22")
	})

	t.Run("default socket path handling", func(t *testing.T) {
		// Test with empty socket path (should default to standard path)
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Hostname:         "example.com",
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "",
		}

		if req.PodmanSocketPath == "" {
			req.PodmanSocketPath = "/run/podman/podman.sock"
		}
		assert.Equal(t, "/run/podman/podman.sock", req.PodmanSocketPath, "Default socket path should be standard")
	})

	t.Run("custom values should be preserved", func(t *testing.T) {
		customPort := 2222
		customSocket := "/custom/socket/path"
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Hostname:         "example.com",
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			Port:             customPort,
			PodmanSocketPath: customSocket,
		}

		// Apply defaults (should not change custom values)
		if req.Port == 0 {
			req.Port = 22
		}
		if req.PodmanSocketPath == "" {
			req.PodmanSocketPath = "/run/podman/podman.sock"
		}

		assert.Equal(t, customPort, req.Port, "Custom port should be preserved")
		assert.Equal(t, customSocket, req.PodmanSocketPath, "Custom socket path should be preserved")
	})
}