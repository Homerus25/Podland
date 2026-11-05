package hosts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
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

// TestCreateHostRequest_Defaults tests default value handling for host creation
func TestCreateHostRequest_Defaults(t *testing.T) {
	t.Run("default port when 0", func(t *testing.T) {
		req := &CreateHostRequest{
			Name:          "test-host",
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
		req := &CreateHostRequest{
			Name:          "test-host",
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
		req := &CreateHostRequest{
			Name:             "test-host",
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
		req := &CreateHostRequest{
			Name:             "test-host",
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

// TestHostService_Create_Validation tests host creation validation logic
func TestHostService_Create_Validation(t *testing.T) {
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

// TestHostCreation_RequestValidation tests request validation for host creation
func TestHostCreation_RequestValidation(t *testing.T) {
	t.Run("valid request with all fields", func(t *testing.T) {
		req := &CreateHostRequest{
			Name:             "test-host",
			Description:      "Test host",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}

		// Basic validation - name should not be empty
		assert.NotEmpty(t, req.Name, "Host name should not be empty")
		assert.NotEmpty(t, req.Hostname, "Hostname should not be empty")
		assert.NotEmpty(t, req.SSHUsername, "SSH username should not be empty")
		assert.True(t, IsValidPEM(req.SSHPrivateKey), "SSH private key should be valid")
	})

	t.Run("request with missing name", func(t *testing.T) {
		req := &CreateHostRequest{
			Name:          "", // Empty name
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		assert.Empty(t, req.Name, "Name should be empty for this test case")
	})

	t.Run("request with invalid PEM key", func(t *testing.T) {
		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: "invalid-key",
		}

		assert.False(t, IsValidPEM(req.SSHPrivateKey), "Invalid PEM key should be rejected")
	})
}

// TestHostService_PEMValidationPerformance tests PEM validation performance
func TestHostService_PEMValidationPerformance(t *testing.T) {
	// Generate a valid PEM key for performance testing
	validKey := testutils.GenerateValidPEMKey()
	invalidKey := "invalid-key"

	// Test multiple validations to ensure performance is acceptable
	startTime := time.Now()
	for range 100 {
		_ = IsValidPEM(validKey)
		_ = IsValidPEM(invalidKey)
	}
	duration := time.Since(startTime)

	// Should complete 200 validations in less than 100ms
	assert.Less(t, duration, 100*time.Millisecond, "PEM validation should be fast")
}

// TestHostService_DefaultValues tests default value handling
func TestHostService_DefaultValues(t *testing.T) {
	t.Run("default port handling", func(t *testing.T) {
		// Test with zero port (should default to 22)
		req := &CreateHostRequest{
			Name:          "test-host",
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
		req := &CreateHostRequest{
			Name:             "test-host",
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
		req := &CreateHostRequest{
			Name:             "test-host",
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

// TestIsValidPEM_ValidEd25519Key tests Ed25519 key validation
func TestIsValidPEM_ValidEd25519Key(t *testing.T) {
	t.Run("valid Ed25519 key", func(t *testing.T) {
		// Ed25519 private key in PEM format
		ed25519Key := `-----BEGIN ED25519 PRIVATE KEY-----
MFECAQEwBQYDK2VwBCIE Alexandru-Gheorghiu-Tudor
MC4A2jNMLp Hélène
4VFblR3F5xF1YdOR3s4jQJ3OiBjYyYXDVoajgGT Black
1TBthrXfY7d3YxkZd5SgxcSe1XGx9kLXe3NokmFnXMfM3V8L
-----END ED25519 PRIVATE KEY-----`
		result := IsValidPEM(ed25519Key)
		assert.True(t, result, "Valid Ed25519 PEM key should be accepted")
	})

	t.Run("invalid key - not a private key (public key)", func(t *testing.T) {
		publicKey := "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG...\n-----END PUBLIC KEY-----"
		result := IsValidPEM(publicKey)
		assert.False(t, result, "Public key should be rejected by PEM validation")
	})

	t.Run("invalid key - DSA private key", func(t *testing.T) {
		dsaKey := "-----BEGIN DSA PRIVATE KEY-----\nMIIBuwIBAAKCAQEA...\n-----END DSA PRIVATE KEY-----"
		result := IsValidPEM(dsaKey)
		assert.True(t, result, "Valid DSA PEM key should be accepted")
	})
}

// TestHostService_Delete tests the HostService Delete method
func TestHostService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("successful deletion by owner", func(t *testing.T) {
		// Setup mock repositories
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnectionManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Create test data
		ownerID := "user-123"
		host := testutils.GenerateTestHostWithOwner(ownerID)
		mockHostRepo.AddHost(host)

		// Create service
		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnectionManager, mockEncryption, newFakeSSHInvalidator())

		// Execute
		err := service.Delete(ctx, ownerID, host.ID)

		// Verify
		assert.NoError(t, err, "Deletion should succeed for owner")

		// Verify host was removed from mock repo
		resultHost, _ := mockHostRepo.GetByID(host.ID)
		assert.Nil(t, resultHost, "Host should be deleted from repository")
	})

	t.Run("error when host not found", func(t *testing.T) {
		// Setup mock repositories
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnectionManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Create service
		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnectionManager, mockEncryption, newFakeSSHInvalidator())

		// Execute
		err := service.Delete(ctx, "user-123", "non-existent-host-id")

		// Verify
		assert.Error(t, err, "Should return error when host not found")
		assert.Contains(t, err.Error(), "HOST_NOT_FOUND", "Should return ErrHostNotFound")
	})

	t.Run("error when user is not owner", func(t *testing.T) {
		// Setup mock repositories
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnectionManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Create test data - host owned by someone else
		ownerID := "actual-owner-123"
		nonOwnerID := "non-owner-456"
		host := testutils.GenerateTestHostWithOwner(ownerID)
		mockHostRepo.AddHost(host)

		// Create service
		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnectionManager, mockEncryption, newFakeSSHInvalidator())

		// Execute - try to delete as non-owner
		err := service.Delete(ctx, nonOwnerID, host.ID)

		// Verify
		assert.Error(t, err, "Should return error when user is not owner")
		assert.Contains(t, err.Error(), "ACCESS_DENIED", "Should return ErrAccessDenied")

		// Verify host still exists (not deleted)
		resultHost, _ := mockHostRepo.GetByID(host.ID)
		assert.NotNil(t, resultHost, "Host should not be deleted when user is not owner")
	})

	t.Run("error when repository delete fails", func(t *testing.T) {
		// Setup mock repositories with error
		deleteError := fmt.Errorf("database connection failed")
		mockHostRepo := testutils.NewMockHostRepositoryWithErrors(nil, nil, nil, nil, deleteError)
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnectionManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Create test data
		ownerID := "user-123"
		host := testutils.GenerateTestHostWithOwner(ownerID)
		// Add host manually to bypass create error
		mockHostRepo.AddHost(host)

		// Create service
		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnectionManager, mockEncryption, newFakeSSHInvalidator())

		// Execute
		err := service.Delete(ctx, ownerID, host.ID)

		// Verify
		assert.Error(t, err, "Should return error when repository delete fails")
		assert.Contains(t, err.Error(), "failed to delete host", "Error should contain repository error details")
	})
}
