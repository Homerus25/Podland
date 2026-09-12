package machines

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
// Full MachineService Tests - Using mock dependencies to test actual service methods
// =============================================================================

// TestMachineService_Create tests the full Create method with mock dependencies
func TestMachineService_Create(t *testing.T) {
	ctx := context.Background()
	userID := "test-user-123"

	t.Run("successful machine creation", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Create request with valid data
		req := &CreateMachineRequest{
			Name:             "test-machine",
			Description:      "Test description",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}

		// Call Create method
		machine, err := service.Create(ctx, userID, req)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, machine)

		// Check machine fields
		assert.Equal(t, "test-machine", machine.Name)
		assert.Equal(t, "Test description", machine.Description)
		assert.Equal(t, "example.com", machine.Hostname)
		assert.Equal(t, 22, machine.Port)
		assert.Equal(t, "root", machine.SSHUsername)
		assert.Equal(t, userID, machine.OwnerID)
		assert.Equal(t, "/run/podman/podman.sock", machine.PodmanSocketPath)
		assert.NotEmpty(t, machine.ID)
		assert.NotEmpty(t, machine.CreatedAt)
		assert.NotEmpty(t, machine.UpdatedAt)
		assert.NotEmpty(t, machine.SSHKeyEncrypted)
		assert.NotEmpty(t, machine.SSHKeyFingerprint)

		// Verify the machine was saved in the repository
		savedMachine, err := mockMachineRepo.GetByID(machine.ID)
		require.NoError(t, err)
		require.NotNil(t, savedMachine)
		assert.Equal(t, machine.ID, savedMachine.ID)

		// Verify access was created
		access, err := mockAccessRepo.GetByMachineAndUser(machine.ID, userID)
		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Equal(t, machine.ID, access.MachineID)
		assert.Equal(t, userID, access.UserID)
		assert.Equal(t, models.PermissionAdmin, access.Permission)
	})

	t.Run("machine creation with default values", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Create request with missing optional fields (should use defaults)
		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
			// Port and PodmanSocketPath should use defaults
		}

		// Call Create method
		machine, err := service.Create(ctx, userID, req)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, machine)

		// Check default values were applied
		assert.Equal(t, 22, machine.Port, "Default port should be 22")
		assert.Equal(t, "/run/podman/podman.sock", machine.PodmanSocketPath, "Default socket path should be /run/podman/podman.sock")
	})

	t.Run("machine creation with missing required fields", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test missing name
		req := &CreateMachineRequest{
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "Missing required fields")

		// Test missing hostname
		req = &CreateMachineRequest{
			Name:          "test-machine",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)

		// Test missing SSH username
		req = &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)

		// Test missing SSH private key
		req = &CreateMachineRequest{
			Name:        "test-machine",
			Hostname:    "example.com",
			SSHUsername: "root",
		}

		machine, err = service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "Missing required fields")
	})

	t.Run("machine creation with invalid SSH key", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: "invalid-ssh-key", // Invalid PEM
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "Invalid SSH private key format")
	})

	t.Run("machine creation with duplicate name", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a machine with the same name first
		existingMachine := &models.Machine{
			ID:        "existing-machine",
			Name:      "test-machine",
			OwnerID:   userID,
			Hostname:  "existing.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockMachineRepo.AddMachine(existingMachine)

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateMachineRequest{
			Name:          "test-machine", // Duplicate name
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "Machine with this name already exists")
	})

	t.Run("machine creation with encryption error", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionServiceWithErrors(errors.New("encryption failed"))

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "failed to encrypt SSH key")
	})

	t.Run("machine creation with database error", func(t *testing.T) {
		// Setup mocks with database error
		createErr := errors.New("database error")
		mockMachineRepo := testutils.NewMockMachineRepositoryWithErrors(nil, nil, createErr, nil, nil)
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "failed to create machine")
	})

	t.Run("machine creation with access creation rollback", func(t *testing.T) {
		// Setup mocks with access creation error
		accessCreateErr := errors.New("access creation error")
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepositoryWithErrors(accessCreateErr, nil, nil, nil, nil, nil, nil)
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		req := &CreateMachineRequest{
			Name:          "test-machine",
			Hostname:      "example.com",
			SSHUsername:   "root",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		}

		machine, err := service.Create(ctx, userID, req)
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "failed to grant owner access")

		// Verify machine was rolled back (deleted)
		machines, _ := mockMachineRepo.ListAccessible(userID)
		assert.Equal(t, 0, len(machines), "Machine should have been rolled back and deleted")
	})
}

// =============================================================================
// GetMachineByID Tests
// =============================================================================

// TestMachineService_GetMachineByID tests the GetMachineByID method
func TestMachineService_GetMachineByID(t *testing.T) {
	userID := "test-user-123"
	machineID := "test-machine-123"

	t.Run("successful retrieval of existing machine", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a machine and access
		testMachine := &models.Machine{
			ID:        machineID,
			Name:      "test-machine",
			OwnerID:   userID,
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockMachineRepo.AddMachine(testMachine)
		mockAccessRepo.AddAccess(machineID, userID, models.PermissionAdmin, time.Now())

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetMachineByID
		machine, err := service.GetMachineByID(userID, machineID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, machineID, machine.ID)
		assert.Equal(t, "test-machine", machine.Name)
		assert.Equal(t, userID, machine.OwnerID)
	})

	t.Run("machine not found", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetMachineByID for non-existent machine
		machine, err := service.GetMachineByID(userID, "non-existent-machine")

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "Machine not found")
	})

	t.Run("user does not have access to machine", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add a machine owned by someone else
		testMachine := &models.Machine{
			ID:        machineID,
			Name:      "other-users-machine",
			OwnerID:   "other-user",
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		mockMachineRepo.AddMachine(testMachine)
		// No access granted for our user

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call GetMachineByID
		machine, err := service.GetMachineByID(userID, machineID)

		// Verify results
		assert.Error(t, err)
		assert.Nil(t, machine)
		assert.Contains(t, err.Error(), "User does not have permission to access this resource")
	})
}

// =============================================================================
// List Methods Tests
// =============================================================================

// TestMachineService_ListAllMachines tests the service's list method
func TestMachineService_ListAllMachines(t *testing.T) {
	userID := "test-user-123"

	t.Run("ListAllMachines", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Add some machines
		machine1 := &models.Machine{
			ID:        "machine-1",
			Name:      "my-machine",
			OwnerID:   userID,
			Hostname:  "host1.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockMachineRepo.AddMachine(machine1)

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Call ListAllMachines
		machines, err := service.ListAllMachines(userID)

		// Verify results
		require.NoError(t, err)
		require.NotNil(t, machines)
		assert.True(t, len(machines) >= 1, "Should return at least one machine")

		// Each entry carries the caller's permission
		assert.Equal(t, models.PermissionAdmin, machines[0].Permission)
	})
}

// =============================================================================
// UserHasAccess Tests
// =============================================================================

// TestMachineService_AccessControl tests the UserHasAccess method
func TestMachineService_AccessControl(t *testing.T) {
	userID := "test-user-123"
	machineID := "test-machine-123"

	t.Run("UserHasAccess - user has access", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// Grant access
		mockAccessRepo.AddAccess(machineID, userID, models.PermissionAdmin, time.Now())

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test UserHasAccess
		hasAccess, err := service.UserHasAccess(userID, machineID)
		require.NoError(t, err)
		assert.True(t, hasAccess, "User should have access to the machine")
	})

	t.Run("UserHasAccess - user has no access records", func(t *testing.T) {
		// Setup mocks
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		// No access granted

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		// Test UserHasAccess
		hasAccess, err := service.UserHasAccess(userID, machineID)
		require.NoError(t, err)
		assert.False(t, hasAccess, "User should not have access to the machine")
	})
}

// =============================================================================
// FromModel Tests
// =============================================================================

// TestMachineService_FromModel tests the FromModel function (moved here for completeness)
func TestMachineService_FromModel_Full(t *testing.T) {
	// This test is also in machine_service_test.go but we include it here for completeness
	// when running all service tests together
	now := time.Now().UTC()

	machine := &models.Machine{
		ID:                "test-machine-123",
		Name:              "Test Machine",
		Description:       "A test machine",
		Hostname:          "example.com",
		Port:              22,
		SSHUsername:       "root",
		SSHKeyEncrypted:   []byte("encrypted-key-data"),
		SSHKeyFingerprint: "sha256:abc123",
		PodmanSocketPath:  "/run/podman/podman.sock",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	response := FromModel(machine)

	// Test all field mappings
	assert.Equal(t, machine.ID, response.ID)
	assert.Equal(t, machine.Name, response.Name)
	assert.Equal(t, machine.Description, response.Description)
	assert.Equal(t, machine.Hostname, response.Hostname)
	assert.Equal(t, machine.Port, response.Port)
	assert.Equal(t, machine.SSHUsername, response.SSHUsername)
	assert.Equal(t, machine.SSHKeyFingerprint, response.SSHKeyFingerprint)
	assert.Equal(t, machine.PodmanSocketPath, response.PodmanSocketPath)
	assert.Equal(t, now.Format(time.RFC3339), response.CreatedAt)
	assert.Equal(t, now.Format(time.RFC3339), response.UpdatedAt)
}
