package hosts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Full HostService Tests - Using mock dependencies to test actual service methods
// =============================================================================

// TestHostService_Create tests the full Create method with mock dependencies
func TestHostService_Create(t *testing.T) {
	ctx := context.Background()
	userID := "test-user-123"

	t.Run("successful host creation", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Create request with valid data
		req := &CreateHostRequest{
			Name:             "test-host",
			Description:      "Test description",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}

		// Call Create method
		host, err := service.Create(ctx, userID, req)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, host)

		// Check host fields
		assert.Equal(t, "test-host", host.Name)
		assert.Equal(t, "Test description", host.Description)
		assert.Equal(t, "example.com", host.Hostname)
		assert.Equal(t, 22, host.Port)
		assert.Equal(t, "root", host.SSHUsername)
		assert.Equal(t, userID, host.OwnerID)
		assert.Equal(t, "/run/podman/podman.sock", host.PodmanSocketPath)
		assert.NotEmpty(t, host.ID)
		assert.NotEmpty(t, host.CreatedAt)
		assert.NotEmpty(t, host.UpdatedAt)
		assert.NotEmpty(t, host.SSHKeyEncrypted)
		assert.NotEmpty(t, host.SSHKeyFingerprint)

		// Verify the host was saved in the repository
		savedHost, err := mockHostRepo.GetByID(host.ID)
		require.NoError(t, err)
		require.NotNil(t, savedHost)
		assert.Equal(t, host.ID, savedHost.ID)

		// Verify access was created
		access, err := mockAccessRepo.GetByHostAndUser(host.ID, userID)
		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Equal(t, host.ID, access.HostID)
		assert.Equal(t, userID, access.UserID)
		assert.Equal(t, models.PermissionAdmin, access.Permission)
	})

	t.Run("host creation with default values", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Create request with missing optional fields (should use defaults)
		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
			// Port and PodmanSocketPath should use defaults
		}

		// Call Create method
		host, err := service.Create(ctx, userID, req)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, host)

		// Check default values were applied
		assert.Equal(t, 22, host.Port, "Default port should be 22")
		assert.Equal(t, "/run/podman/podman.sock", host.PodmanSocketPath, "Default socket path should be /run/podman/podman.sock")
	})

	t.Run("host creation with missing required fields", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test missing name
		req := &CreateHostRequest{
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "Missing required fields")
		assert.Contains(t, err.Error(), "name",
			"the message must list the missing field names (issue 399)")

		// Test missing hostname
		req = &CreateHostRequest{
			Name:          "test-host",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "hostname",
			"the message must list the missing field names (issue 399)")

		// Test missing SSH username
		req = &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "sshUsername",
			"the message must list the missing field names (issue 399)")

		// Test missing SSH private key
		req = &CreateHostRequest{
			Name:        "test-host",
			Hostname:    "example.com",
			SSHUsername: "root",
		}

		host, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "Missing required fields")
		assert.Contains(t, err.Error(), "sshPrivateKey",
			"the message must list the missing field names (issue 399)")
	})

	t.Run("host creation with invalid SSH key", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: "invalid-ssh-key", // Invalid PEM
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "Invalid SSH private key format")
		assert.Contains(t, err.Error(), "PRIVATE KEY",
			"the message must describe the expected PEM marker format (issue 399)")
	})

	t.Run("host creation with duplicate name", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a host with the same name first
		existingHost := &models.Host{
			ID:        "existing-host",
			Name:      "test-host",
			OwnerID:   userID,
			Hostname:  "existing.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockHostRepo.AddHost(existingHost)

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateHostRequest{
			Name:          "test-host", // Duplicate name
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "Host with name")
		assert.Contains(t, err.Error(), `"test-host"`,
			"the message must name the taken host name (issue 399)")
	})

	t.Run("host creation with encryption error", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionServiceWithErrors(errors.New("encryption failed"))

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "failed to encrypt SSH key")
	})

	t.Run("host creation with database error", func(t *testing.T) {
		// Setup mocks with database error
		createErr := errors.New("database error")
		mockHostRepo := testutils.NewMockHostRepositoryWithErrors(nil, nil, createErr, nil, nil)
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "failed to create host")
	})

	t.Run("host creation with access creation rollback", func(t *testing.T) {
		// Setup mocks with access creation error
		accessCreateErr := errors.New("access creation error")
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepositoryWithErrors(accessCreateErr, nil, nil, nil, nil, nil, nil)
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateHostRequest{
			Name:          "test-host",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		host, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "failed to grant owner access")

		// Verify host was rolled back (deleted)
		hosts, _ := mockHostRepo.ListAccessible(userID)
		assert.Equal(t, 0, len(hosts), "Host should have been rolled back and deleted")
	})
}

// =============================================================================
// GetHostByID Tests
// =============================================================================

// TestHostService_GetHostByID tests the GetHostByID method
func TestHostService_GetHostByID(t *testing.T) {
	userID := "test-user-123"
	hostID := "test-host-123"

	t.Run("successful retrieval of existing host", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a host and access
		testHost := &models.Host{
			ID:        hostID,
			Name:      "test-host",
			OwnerID:   userID,
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockHostRepo.AddHost(testHost)
		mockAccessRepo.AddAccess(hostID, userID, models.PermissionAdmin, time.Now())

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetHostByID
		accessible, err := service.GetHostByID(userID, hostID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, accessible)
		host := accessible.Host
		assert.Equal(t, hostID, host.ID)
		assert.Equal(t, "test-host", host.Name)
		assert.Equal(t, userID, host.OwnerID)
		// The caller's stored level rides the answer: the detail
		// endpoint enriches its response from it.
		assert.Equal(t, models.PermissionAdmin, accessible.Permission)
	})

	t.Run("grant retrieval carries the granted level", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// A host owned by someone else, granted to our user at read.
		testHost := &models.Host{
			ID:        hostID,
			Name:      "shared-host",
			OwnerID:   "other-user",
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockHostRepo.AddHost(testHost)
		mockAccessRepo.AddAccess(hostID, userID, models.PermissionRead, time.Now())

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		accessible, err := service.GetHostByID(userID, hostID)

		require.NoError(t, err)
		require.NotNil(t, accessible)
		assert.Equal(t, models.PermissionRead, accessible.Permission)
		assert.NotEqual(t, userID, accessible.OwnerID, "a grantee is not the owner")
	})

	t.Run("host not found", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetHostByID for non-existent host
		host, err := service.GetHostByID(userID, "non-existent-host")

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "Host not found")
	})

	t.Run("user does not have access to host", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a host owned by someone else
		testHost := &models.Host{
			ID:        hostID,
			Name:      "other-users-host",
			OwnerID:   "other-user",
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockHostRepo.AddHost(testHost)
		// No access granted for our user

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetHostByID
		host, err := service.GetHostByID(userID, hostID)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, host)
		assert.Contains(t, err.Error(), "User does not have permission to access this resource")
	})
}

// =============================================================================
// List Methods Tests
// =============================================================================

// TestHostService_ListAllHosts tests the service's list method
func TestHostService_ListAllHosts(t *testing.T) {
	userID := "test-user-123"

	t.Run("ListAllHosts", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add some hosts
		host1 := &models.Host{
			ID:        "host-1",
			Name:      "my-host",
			OwnerID:   userID,
			Hostname:  "host1.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockHostRepo.AddHost(host1)

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call ListAllHosts
		hosts, err := service.ListAllHosts(userID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, hosts)
		assert.True(t, len(hosts) >= 1, "Should return at least one host")

		// Each entry carries the caller's permission
		assert.Equal(t, models.PermissionAdmin, hosts[0].Permission)
	})
}

// =============================================================================
// UserHasAccess Tests
// =============================================================================

// TestHostService_AccessControl tests the UserHasAccess method
func TestHostService_AccessControl(t *testing.T) {
	userID := "test-user-123"
	hostID := "test-host-123"

	t.Run("UserHasAccess - user has access", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Grant access
		mockAccessRepo.AddAccess(hostID, userID, models.PermissionAdmin, time.Now())

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test UserHasAccess
		hasAccess, err := service.UserHasAccess(userID, hostID)
		require.NoError(t, err)
		assert.True(t, hasAccess, "User should have access to the host")
	})

	t.Run("UserHasAccess - user has no access records", func(t *testing.T) {
		// Setup mocks
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// No access granted

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test UserHasAccess
		hasAccess, err := service.UserHasAccess(userID, hostID)
		require.NoError(t, err)
		assert.False(t, hasAccess, "User should not have access to the host")
	})
}

// =============================================================================
// FromModel Tests
// =============================================================================

// TestHostService_FromModel tests the FromModel function (moved here for completeness)
func TestHostService_FromModel_Full(t *testing.T) {
	// This test is also in host_service_test.go but we include it here for completeness
	// when running all service tests together
	now := time.Now().UTC()

	host := &models.Host{
		ID:                "test-host-123",
		Name:              "Test Host",
		Description:       "A test host",
		Hostname:          "example.com",
		Port:              22,
		SSHUsername:       "root",
		SSHKeyEncrypted:   []byte("encrypted-key-data"),
		SSHKeyFingerprint: "sha256:abc123",
		PodmanSocketPath:  "/run/podman/podman.sock",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	response := FromModel(host)

	// Test all field mappings
	assert.Equal(t, host.ID, response.ID)
	assert.Equal(t, host.Name, response.Name)
	assert.Equal(t, host.Description, response.Description)
	assert.Equal(t, host.Hostname, response.Hostname)
	assert.Equal(t, host.Port, response.Port)
	assert.Equal(t, host.SSHUsername, response.SSHUsername)
	assert.Equal(t, host.SSHKeyFingerprint, response.SSHKeyFingerprint)
	assert.Equal(t, host.PodmanSocketPath, response.PodmanSocketPath)
	assert.Equal(t, now.Format(time.RFC3339), response.CreatedAt)
	assert.Equal(t, now.Format(time.RFC3339), response.UpdatedAt)
}
