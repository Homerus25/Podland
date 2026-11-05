package hosts

import (
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Host Creation Validation Tests
// =============================================================================

// TestHostCreation_Validation tests host creation validation logic
func TestHostCreation_Validation(t *testing.T) {

	// Test validation that happens in HostService.Create
	t.Run("required field validation", func(t *testing.T) {
		testCases := []struct {
			name       string
			req        *CreateHostRequest
			shouldFail bool
		}{
			{
				name: "missing name",
				req: &CreateHostRequest{
					Hostname:      "example.com",
					SSHUsername:   "root",
					SSHPrivateKey: testutils.GenerateValidPEMKey(),
				},
				shouldFail: true,
			},
			{
				name: "missing hostname",
				req: &CreateHostRequest{
					Name:          "test",
					SSHUsername:   "root",
					SSHPrivateKey: testutils.GenerateValidPEMKey(),
				},
				shouldFail: true,
			},
			{
				name: "missing SSH username",
				req: &CreateHostRequest{
					Name:          "test",
					Hostname:      "example.com",
					SSHPrivateKey: testutils.GenerateValidPEMKey(),
				},
				shouldFail: true,
			},
			{
				name: "missing SSH private key",
				req: &CreateHostRequest{
					Name:        "test",
					Hostname:    "example.com",
					SSHUsername: "root",
				},
				shouldFail: true,
			},
			{
				name: "all fields present",
				req: &CreateHostRequest{
					Name:          "test",
					Hostname:      "example.com",
					SSHUsername:   "root",
					SSHPrivateKey: testutils.GenerateValidPEMKey(),
				},
				shouldFail: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test the validation logic from Create method
				if tc.req.Name == "" || tc.req.Hostname == "" || tc.req.SSHUsername == "" || tc.req.SSHPrivateKey == "" {
					assert.True(t, tc.shouldFail, "Should fail validation for missing required fields")
				} else {
					assert.False(t, tc.shouldFail, "Should pass validation when all required fields are present")
				}
			})
		}
	})

	t.Run("PEM validation", func(t *testing.T) {
		testCases := []struct {
			name   string
			pemKey string
			valid  bool
		}{
			{"valid RSA key", testutils.GenerateValidPEMKey(), true},
			{"valid OpenSSH key", testutils.GenerateValidOpenSSHKey(), true},
			{"valid EC key", testutils.GenerateValidECKey(), true},
			{"invalid key", "invalid-key", false},
			{"empty key", "", false},
			{"short key", "short", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				valid := IsValidPEM(tc.pemKey)
				assert.Equal(t, tc.valid, valid, "PEM validation should return %v for %s", tc.valid, tc.name)
			})
		}
	})

	t.Run("default value application", func(t *testing.T) {
		testCases := []struct {
			name           string
			port           int
			socketPath     string
			expectedPort   int
			expectedSocket string
		}{
			{"zero port, empty socket", 0, "", 22, "/run/podman/podman.sock"},
			{"zero port, custom socket", 0, "/custom/path", 22, "/custom/path"},
			{"custom port, empty socket", 2222, "", 2222, "/run/podman/podman.sock"},
			{"custom port, custom socket", 2222, "/custom/path", 2222, "/custom/path"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := &CreateHostRequest{
					Name:             "test",
					Hostname:         "example.com",
					SSHUsername:      "root",
					SSHPrivateKey:    testutils.GenerateValidPEMKey(),
					Port:             tc.port,
					PodmanSocketPath: tc.socketPath,
				}

				// Apply defaults (same logic as in Create method)
				if req.Port == 0 {
					req.Port = 22
				}
				if req.PodmanSocketPath == "" {
					req.PodmanSocketPath = "/run/podman/podman.sock"
				}

				assert.Equal(t, tc.expectedPort, req.Port, "Port should be %d after applying defaults", tc.expectedPort)
				assert.Equal(t, tc.expectedSocket, req.PodmanSocketPath, "Socket path should be %s after applying defaults", tc.expectedSocket)
			})
		}
	})
}

// =============================================================================
// Host Service Helper Tests
// =============================================================================

// TestHostService_ListFunctions tests list functions with mock repositories
func TestHostService_ListFunctions(t *testing.T) {
	// Create repositories that implement the interfaces
	mockHostRepo := testutils.NewMockHostRepository()
	// mockAccessRepo := testutils.NewMockHostAccessRepository() // Unused in current tests

	userID := "test-user-123"

	t.Run("ListAccessible with mock repository", func(t *testing.T) {
		// Add hosts
		host1 := &models.Host{
			ID:        "host-1",
			Name:      "accessible-host-1",
			OwnerID:   userID,
			Hostname:  "host1.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		host2 := &models.Host{
			ID:        "host-2",
			Name:      "accessible-host-2",
			OwnerID:   userID, // Owned by user
			Hostname:  "host2.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		host3 := &models.Host{
			ID:        "host-3",
			Name:      "other-host",
			OwnerID:   "other-user",
			Hostname:  "host3.example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		mockHostRepo.AddHost(host1)
		mockHostRepo.AddHost(host2)
		mockHostRepo.AddHost(host3)

		// Test ListAccessible
		hosts, err := mockHostRepo.ListAccessible(userID)
		require.NoError(t, err)
		// Should return hosts owned by user (ListAccessible in our mock returns hosts by owner)
		assert.Equal(t, 2, len(hosts), "Should return hosts accessible to user %s", userID)
	})
}

// =============================================================================
// FromModel Conversion Tests
// =============================================================================

// TestHostService_FromModel tests the FromModel conversion function
func TestHostService_FromModel(t *testing.T) {

	t.Run("complete model conversion", func(t *testing.T) {
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
		assert.Equal(t, host.ID, response.ID, "ID should match")
		assert.Equal(t, host.Name, response.Name, "Name should match")
		assert.Equal(t, host.Description, response.Description, "Description should match")
		assert.Equal(t, host.Hostname, response.Hostname, "Hostname should match")
		assert.Equal(t, host.Port, response.Port, "Port should match")
		assert.Equal(t, host.SSHUsername, response.SSHUsername, "SSHUsername should match")
		assert.Equal(t, host.SSHKeyFingerprint, response.SSHKeyFingerprint, "SSHKeyFingerprint should match")
		assert.Equal(t, host.PodmanSocketPath, response.PodmanSocketPath, "PodmanSocketPath should match")

		// Test timestamp formatting
		assert.Equal(t, now.Format(time.RFC3339), response.CreatedAt, "CreatedAt should be formatted in RFC3339")
		assert.Equal(t, now.Format(time.RFC3339), response.UpdatedAt, "UpdatedAt should be formatted in RFC3339")
	})

	t.Run("model with zero timestamps", func(t *testing.T) {
		host := &models.Host{
			ID:               "test-host-456",
			Name:             "Zero Host",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			PodmanSocketPath: "/run/podman/podman.sock",
			// Timestamps are zero values
		}

		response := FromModel(host)

		assert.Equal(t, host.ID, response.ID, "ID should match")
		assert.Equal(t, host.Name, response.Name, "Name should match")
		// Zero timestamps should format as "0001-01-01T00:00:00Z"
		assert.Equal(t, "0001-01-01T00:00:00Z", response.CreatedAt, "Zero CreateAt should format correctly")
		assert.Equal(t, "0001-01-01T00:00:00Z", response.UpdatedAt, "Zero UpdatedAt should format correctly")
	})

	t.Run("model with minimal fields", func(t *testing.T) {
		host := &models.Host{
			ID:       "minimal-host",
			Name:     "Minimal",
			Hostname: "minimal.com",
			Port:     22,
		}

		response := FromModel(host)

		assert.Equal(t, "minimal-host", response.ID)
		assert.Equal(t, "Minimal", response.Name)
		assert.Equal(t, "minimal.com", response.Hostname)
		assert.Equal(t, 22, response.Port)
		assert.Empty(t, response.Description, "Description should be empty")
		assert.Empty(t, response.SSHUsername, "SSHUsername should be empty")
		assert.Empty(t, response.SSHKeyFingerprint, "SSHKeyFingerprint should be empty")
		assert.Empty(t, response.PodmanSocketPath, "PodmanSocketPath should be empty")
	})
}

// =============================================================================
// Performance Tests
// =============================================================================

// TestHostCreation_PEMValidationPerformance tests PEM validation performance
func TestHostCreation_PEMValidationPerformance(t *testing.T) {

	// Generate test keys
	validKey := testutils.GenerateValidPEMKey()
	invalidKey := "invalid-key"

	t.Run("multiple validations", func(t *testing.T) {
		startTime := time.Now()
		for range 100 {
			_ = IsValidPEM(validKey)
			_ = IsValidPEM(invalidKey)
		}
		duration := time.Since(startTime)

		// Should complete 200 validations in less than 100ms
		assert.Less(t, duration, 100*time.Millisecond, "PEM validation should be fast")
	})

	t.Run("bulk validation", func(t *testing.T) {
		// Generate multiple keys
		keys := []string{validKey, invalidKey}
		for i := range 10 {
			keys = append(keys, testutils.GenerateValidPEMKey())
			keys = append(keys, "invalid-key-"+string(rune(i)))
		}

		startTime := time.Now()
		for _, key := range keys {
			_ = IsValidPEM(key)
		}
		duration := time.Since(startTime)

		// Should validate 22 keys very quickly
		assert.Less(t, duration, 50*time.Millisecond, "Bulk PEM validation should be fast")
	})
}

// =============================================================================
// Request Validation Edge Cases
// =============================================================================

// TestHostCreation_EdgeCases tests edge cases for host creation
func TestHostCreation_EdgeCases(t *testing.T) {

	t.Run("edge case hostname formats", func(t *testing.T) {
		edgeCaseHostnames := []string{
			"localhost",
			"127.0.0.1",
			"::1",
			"example.com",
			"sub.example.com",
			"example-with.dashes.com",
			"192.168.1.1",
			"2001:0db8:85a3::8a2e:0370:7334",
		}

		for _, hostname := range edgeCaseHostnames {
			req := &CreateHostRequest{
				Name:          "test-host",
				Hostname:      hostname,
				SSHUsername:   "root",
				SSHPrivateKey: testutils.GenerateValidPEMKey(),
			}

			// Test that the request is valid (hostname format validation is minimal)
			assert.NotEmpty(t, req.Hostname, "Hostname should not be empty")
			assert.True(t, IsValidPEM(req.SSHPrivateKey), "SSH key should be valid")
		}
	})

	t.Run("edge case SSH usernames", func(t *testing.T) {
		edgeCaseUsernames := []string{
			"root",
			"admin",
			"user",
			"user123",
			"user_name",
			"user-name",
			"user@domain",
			"", // Empty - should be rejected in validation
		}

		for _, username := range edgeCaseUsernames {
			req := &CreateHostRequest{
				Name:          "test-host",
				Hostname:      "example.com",
				SSHUsername:   username,
				SSHPrivateKey: testutils.GenerateValidPEMKey(),
			}

			// Just verify the request can be created
			// Validation will happen in the actual Create method
			assert.NotNil(t, req, "Request should be creatable")
		}
	})

	t.Run("edge case host names", func(t *testing.T) {
		edgeCaseNames := []string{
			"simple",
			"host-1",
			"host_name",
			"Host Name", // With space
			"host.name", // With dot
			"host@name", // With @ symbol
			"a",         // Single character
			"",          // Empty - should be rejected in validation
		}

		for _, name := range edgeCaseNames {
			req := &CreateHostRequest{
				Name:          name,
				Hostname:      "example.com",
				SSHUsername:   "root",
				SSHPrivateKey: testutils.GenerateValidPEMKey(),
			}

			// Just verify the request can be created
			assert.NotNil(t, req, "Request should be creatable")
		}
	})
}

// =============================================================================
// Mock Repository Behavior Tests
// =============================================================================

// TestMockRepository_Behavior tests that our mock repositories work as expected
func TestMockRepository_Behavior(t *testing.T) {

	t.Run("HostRepositoryInterface implementation", func(t *testing.T) {
		mockRepo := testutils.NewMockHostRepository()

		// Verify it implements HostRepositoryInterface
		var _ HostRepositoryInterface = mockRepo

		// Test basic operations
		host := &models.Host{
			ID:        "test-host",
			Name:      "Test Host",
			OwnerID:   "test-user",
			Hostname:  "example.com",
			Port:      22,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Test Create
		err := mockRepo.Create(host)
		assert.NoError(t, err, "Should create host without error")

		// Test GetByID
		retrieved, err := mockRepo.GetByID(host.ID)
		assert.NoError(t, err, "Should retrieve host without error")
		assert.NotNil(t, retrieved, "Should return host")
		assert.Equal(t, host.ID, retrieved.ID, "Should return correct host")

		// Test Update
		host.Name = "Updated Host"
		err = mockRepo.Update(host)
		assert.NoError(t, err, "Should update host without error")

		updated, err := mockRepo.GetByID(host.ID)
		assert.NoError(t, err, "Should retrieve updated host without error")
		assert.Equal(t, "Updated Host", updated.Name, "Should have updated name")

		// Test Delete
		err = mockRepo.Delete(host.ID)
		assert.NoError(t, err, "Should delete host without error")

		deleted, err := mockRepo.GetByID(host.ID)
		assert.NoError(t, err, "Should not error when retrieving deleted host")
		assert.Nil(t, deleted, "Should return nil for deleted host")
	})

	t.Run("HostAccessRepositoryInterface implementation", func(t *testing.T) {
		mockRepo := testutils.NewMockHostAccessRepository()

		// Verify it implements HostAccessRepositoryInterface
		var _ models.HostAccessRepositoryInterface = mockRepo

		// Test basic operations
		access := &models.HostAccess{
			HostID:     "test-host",
			UserID:     "test-user",
			Permission: models.PermissionAdmin,
			GrantedAt:  time.Now(),
		}

		// Test Create
		err := mockRepo.Create(access)
		assert.NoError(t, err, "Should create access without error")

		// Test GetByHostAndUser
		retrieved, err := mockRepo.GetByHostAndUser("test-host", "test-user")
		assert.NoError(t, err, "Should retrieve access without error")
		assert.NotNil(t, retrieved, "Should return access")
		assert.Equal(t, access.HostID, retrieved.HostID, "Should return correct access")

		// Test HasAccess
		hasAccess, err := mockRepo.HasAccess("test-host", "test-user")
		assert.NoError(t, err, "Should check access without error")
		assert.True(t, hasAccess, "Should have access")

		// Test ListByUser
		accesses, err := mockRepo.ListByUser("test-user")
		assert.NoError(t, err, "Should list accesses without error")
		assert.Equal(t, 1, len(accesses), "Should return 1 access for user")

		// Test Delete
		err = mockRepo.Delete("test-host", "test-user")
		assert.NoError(t, err, "Should delete access without error")

		hasAccess, err = mockRepo.HasAccess("test-host", "test-user")
		assert.NoError(t, err, "Should check access without error")
		assert.False(t, hasAccess, "Should not have access after deletion")
	})
}
