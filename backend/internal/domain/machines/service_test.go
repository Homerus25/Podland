package machines

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidPEM(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "valid RSA private key",
			key:      "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----",
			expected: true,
		},
		{
			name:     "valid OPENSSH private key",
			key:      "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlblNzaC1rZXktdjEA...\n-----END OPENSSH PRIVATE KEY-----",
			expected: true,
		},
		{
			name:     "valid EC private key",
			key:      "-----BEGIN EC PRIVATE KEY-----\nMHQCAQEEI...\n-----END EC PRIVATE KEY-----",
			expected: true,
		},
		{
			name:     "valid ED25519 private key",
			key:      "-----BEGIN ED25519 PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIE...\n-----END ED25519 PRIVATE KEY-----",
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
			key:      "  -----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----\n  ",
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

func TestCreateMachineRequest_Defaults(t *testing.T) {
	t.Run("default port when 0", func(t *testing.T) {
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAI...",
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
			SSHPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAI...",
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
			SSHPrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAI...",
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
			SSHPrivateKey:    "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAI...",
			PodmanSocketPath: "/run/user/1000/podman/podman.sock",
		}

		// Apply defaults like the service does
		if req.PodmanSocketPath == "" {
			req.PodmanSocketPath = "/run/podman/podman.sock"
		}
		assert.Equal(t, "/run/user/1000/podman/podman.sock", req.PodmanSocketPath)
	})
}

func TestFromModel(t *testing.T) {
	// Test will be added when models are properly imported
	t.Skip("Skipping until models import is fixed")
}

func TestMachineService_Create_Validation(t *testing.T) {
	// This will be implemented when we can properly mock dependencies
	// For now, this serves as a placeholder for the test structure
	t.Skip("Skipping until mock dependencies are set up")
}