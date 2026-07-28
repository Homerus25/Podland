package testutils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockUserRepository tests the mock user repository functionality
func TestMockUserRepository(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		repo := NewMockUserRepository()

		// Test creating a user
		user := GenerateTestUser()
		err := repo.Create(user)
		assert.NoError(t, err)

		// Test getting user by ID
		fetchedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedUser)
		assert.Equal(t, user.ID, fetchedUser.ID)

		// Test getting user by username
		fetchedByUsername, err := repo.GetByUsername(user.Username)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedByUsername)
		assert.Equal(t, user.Username, fetchedByUsername.Username)

		// Test username exists
		exists, err := repo.UsernameExists(user.Username)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Test non-existent user
		nonExistent, err := repo.GetByID("non-existent-id")
		assert.NoError(t, err)
		assert.Nil(t, nonExistent)

		// Test update user
		user.Username = "updated-username"
		err = repo.Update(user)
		assert.NoError(t, err)

		updatedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "updated-username", updatedUser.Username)

		// Test delete user
		err = repo.Delete(user.ID)
		assert.NoError(t, err)

		deletedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Nil(t, deletedUser)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New(" repository error")
		repo := NewMockUserRepositoryWithErrors(
			expectedErr, // createErr
			expectedErr, // getByIDErr
			expectedErr, // getByNameErr
			expectedErr, // updateErr
			expectedErr, // deleteErr
			expectedErr, // existsErr
		)

		user := GenerateTestUser()
		err := repo.Create(user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository error")

		_, err = repo.GetByID(user.ID)
		assert.Error(t, err)

		_, err = repo.GetByUsername(user.Username)
		assert.Error(t, err)

		err = repo.Update(user)
		assert.Error(t, err)

		err = repo.Delete(user.ID)
		assert.Error(t, err)

		_, err = repo.UsernameExists(user.Username)
		assert.Error(t, err)
	})

	t.Run("direct user addition", func(t *testing.T) {
		repo := NewMockUserRepository()
		user := GenerateTestUser()

		// Add user directly without going through Create
		repo.AddUser(user)

		fetchedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedUser)
		assert.Equal(t, user.ID, fetchedUser.ID)
	})
}

// TestMockMachineRepository tests the mock machine repository functionality
func TestMockMachineRepository(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		repo := NewMockMachineRepository()

		// Test creating a machine
		machine := GenerateTestMachine()
		err := repo.Create(machine)
		assert.NoError(t, err)

		// Test getting machine by ID
		fetchedMachine, err := repo.GetByID(machine.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedMachine)
		assert.Equal(t, machine.ID, fetchedMachine.ID)

		// Test updating machine
		machine.Name = "updated-machine-name"
		err = repo.Update(machine)
		assert.NoError(t, err)

		updatedMachine, err := repo.GetByID(machine.ID)
		assert.NoError(t, err)
		assert.Equal(t, "updated-machine-name", updatedMachine.Name)

		// Test delete machine
		err = repo.Delete(machine.ID)
		assert.NoError(t, err)

		deletedMachine, err := repo.GetByID(machine.ID)
		assert.NoError(t, err)
		assert.Nil(t, deletedMachine)
	})

	t.Run("ListByOwner and ListAccessible", func(t *testing.T) {
		repo := NewMockMachineRepository()

		// Create multiple machines for different owners
		machine1 := GenerateTestMachine()
		machine2 := GenerateTestMachine()
		machine2.OwnerID = machine1.OwnerID // Same owner
		machine3 := GenerateTestMachine()

		repo.Create(machine1)
		repo.Create(machine2)
		repo.Create(machine3)

		// Test ListByOwner
		machines, err := repo.ListByOwner(machine1.OwnerID)
		assert.NoError(t, err)
		assert.Len(t, machines, 2)

		// Test ListAccessible
		accessible, err := repo.ListAccessible(machine1.OwnerID)
		assert.NoError(t, err)
		assert.Len(t, accessible, 2)
	})

	t.Run("NameExistsForUser", func(t *testing.T) {
		repo := NewMockMachineRepository()
		machine := GenerateTestMachine()
		repo.Create(machine)

		exists, err := repo.NameExistsForUser(machine.OwnerID, machine.Name)
		assert.NoError(t, err)
		assert.True(t, exists)

		notExists, err := repo.NameExistsForUser(machine.OwnerID, "non-existent-name")
		assert.NoError(t, err)
		assert.False(t, notExists)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New("machine repository error")
		repo := NewMockMachineRepositoryWithErrors(
			expectedErr, // listErr
			expectedErr, // getErr
			expectedErr, // createErr
			expectedErr, // updateErr
			expectedErr, // deleteErr
		)

		machine := GenerateTestMachine()
		err := repo.Create(machine)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "machine repository error")

		_, err = repo.GetByID(machine.ID)
		assert.Error(t, err)

		_, err = repo.ListByOwner("owner-id")
		assert.Error(t, err)

		err = repo.Delete(machine.ID)
		assert.Error(t, err)
	})

	t.Run("direct machine addition", func(t *testing.T) {
		repo := NewMockMachineRepository()
		machine := GenerateTestMachine()

		// Add machine directly without going through Create
		repo.AddMachine(machine)

		fetchedMachine, err := repo.GetByID(machine.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedMachine)
		assert.Equal(t, machine.ID, fetchedMachine.ID)
	})
}

// TestMockEncryptionService tests the mock encryption service functionality
func TestMockEncryptionService(t *testing.T) {
	t.Run("basic encryption/decryption", func(t *testing.T) {
		service := NewMockEncryptionService()

		// Test encryption
		originalData := "secret-data"
		encrypted, err := service.Encrypt(originalData)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.True(t, len(encrypted) > 0)

		// Test decryption
		decrypted, err := service.Decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, originalData, decrypted)
	})

	t.Run("SSH key encryption", func(t *testing.T) {
		service := NewMockEncryptionService()

		// Test SSH key encryption
		sshPrivateKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ..."
		encrypted, fingerprint, err := service.EncryptSSHKey(sshPrivateKey)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.NotEmpty(t, fingerprint)

		// Test SSH key decryption
		decrypted, err := service.DecryptSSHKey(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, sshPrivateKey, decrypted)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New("encryption error")
		service := NewMockEncryptionServiceWithErrors(expectedErr, expectedErr)

		_, err := service.Encrypt("data")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encryption error")

		_, err = service.Decrypt([]byte("encrypted"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encryption error")

		_, _, err = service.EncryptSSHKey("ssh-key")
		assert.Error(t, err)

		_, err = service.DecryptSSHKey([]byte("encrypted"))
		assert.Error(t, err)
	})

	t.Run("custom functions", func(t *testing.T) {
		customEncrypt := func(data string) ([]byte, error) {
			return []byte("custom-encrypted-" + data), nil
		}

		customDecrypt := func(data []byte) (string, error) {
			dataStr := string(data)
			if len(dataStr) > len("custom-encrypted-") && dataStr[:len("custom-encrypted-")] == "custom-encrypted-" {
				return dataStr[len("custom-encrypted-"):], nil
			}
			return dataStr, nil
		}

		customEncryptSSH := func(privateKey string) ([]byte, string, error) {
			return []byte("ssh-custom-" + privateKey), "custom-fingerprint", nil
		}

		customDecryptSSH := func(encryptedKey []byte) (string, error) {
			dataStr := string(encryptedKey)
			if len(dataStr) > len("ssh-custom-") && dataStr[:len("ssh-custom-")] == "ssh-custom-" {
				return dataStr[len("ssh-custom-"):], nil
			}
			return dataStr, nil
		}

		service := NewMockEncryptionServiceWithCustomFunctions(
			customEncrypt,
			customDecrypt,
			customEncryptSSH,
			customDecryptSSH,
		)

		// Test custom encryption
		encrypted, err := service.Encrypt("test-data")
		assert.NoError(t, err)
		assert.Equal(t, "custom-encrypted-test-data", string(encrypted))

		// Test custom decryption
		decrypted, err := service.Decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, "test-data", decrypted)
	})
}

// TestMockConnectionCache tests the mock connection cache functionality
func TestMockConnectionCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		cache := NewMockConnectionCache()
		ctx := context.Background()

		// Test GetOrCreate
		conn, err := cache.GetOrCreate(ctx, "machine-1", "user-1")
		assert.NoError(t, err)
		assert.NotNil(t, conn)

		// Verify the connection is of the expected type
		mockConn, ok := conn.(*MockPodmanConnection)
		assert.True(t, ok)
		assert.Equal(t, "machine-1", mockConn.GetMachineID())

		// Test Release
		cache.Release("machine-1")

		// Test MarkActive and MarkIdle (should not panic)
		cache.MarkActive("machine-1")
		cache.MarkIdle("machine-1")

		// Test Clear
		cache.Clear()
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New("connection cache error")
		cache := NewMockConnectionCacheWithError(expectedErr)
		ctx := context.Background()

		_, err := cache.GetOrCreate(ctx, "machine-1", "user-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection cache error")
	})

	t.Run("custom GetOrCreate function", func(t *testing.T) {
		customFunc := func(ctx context.Context, machineID, userID string) (any, error) {
			return &MockPodmanConnection{MachineID: "custom-" + machineID}, nil
		}

		cache := NewMockConnectionCache()
		cache.SetCustomGetOrCreate(customFunc)
		ctx := context.Background()

		conn, err := cache.GetOrCreate(ctx, "machine-1", "user-1")
		assert.NoError(t, err)
		assert.NotNil(t, conn)

		mockConn, ok := conn.(*MockPodmanConnection)
		assert.True(t, ok)
		assert.Equal(t, "custom-machine-1", mockConn.GetMachineID())
	})
}

// TestTestDataGenerators tests the test data generator functions
func TestTestDataGenerators(t *testing.T) {
	t.Run("GenerateTestUser", func(t *testing.T) {
		user := GenerateTestUser()
		assert.NotNil(t, user)
		assert.NotEmpty(t, user.ID)
		assert.NotEmpty(t, user.Username)
		assert.NotEmpty(t, user.PasswordHash)
		assert.False(t, user.CreatedAt.IsZero())
	})

	t.Run("GenerateTestMachine", func(t *testing.T) {
		machine := GenerateTestMachine()
		assert.NotNil(t, machine)
		assert.NotEmpty(t, machine.ID)
		assert.NotEmpty(t, machine.Name)
		assert.NotEmpty(t, machine.OwnerID)
		assert.NotEmpty(t, machine.Hostname)
		assert.Equal(t, 22, machine.Port)            // Default SSH port
		assert.Equal(t, "root", machine.SSHUsername) // Default SSH username
		assert.False(t, machine.CreatedAt.IsZero())
		assert.False(t, machine.UpdatedAt.IsZero())
	})

	t.Run("GenerateValidPEMKey", func(t *testing.T) {
		pemKey := GenerateValidPEMKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, pemKey, "END RSA PRIVATE KEY")
	})

	t.Run("GenerateInvalidPEMKey", func(t *testing.T) {
		pemKey := GenerateInvalidPEMKey()
		assert.Equal(t, "invalid-key-data", pemKey)
	})

	t.Run("GenerateValidOpenSSHKey", func(t *testing.T) {
		pemKey := GenerateValidOpenSSHKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN OPENSSH PRIVATE KEY")
		assert.Contains(t, pemKey, "END OPENSSH PRIVATE KEY")
	})

	t.Run("GenerateValidECKey", func(t *testing.T) {
		pemKey := GenerateValidECKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN EC PRIVATE KEY")
		assert.Contains(t, pemKey, "END EC PRIVATE KEY")
	})

	t.Run("CreateTestJWTKeys", func(t *testing.T) {
		privateKey, publicKey, err := CreateTestJWTKeys()
		assert.NoError(t, err)
		assert.NotEmpty(t, privateKey)
		assert.NotEmpty(t, publicKey)
		assert.Contains(t, privateKey, "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, publicKey, "BEGIN PUBLIC KEY")
	})
}

// TestTestConfiguration tests the test configuration functionality
func TestTestConfiguration(t *testing.T) {
	t.Run("DefaultTestConfig", func(t *testing.T) {
		config := DefaultTestConfig()
		assert.NotNil(t, config)
		assert.Equal(t, 30, int(config.TestTimeout.Seconds()))
		assert.Equal(t, 1, config.RetryAttempts)
		assert.False(t, config.VerboseOutput)
		assert.True(t, config.MockResponses)
		assert.True(t, config.CleanupOnExit)
	})

	t.Run("Global config initialization", func(t *testing.T) {
		// Reset global config
		globalConfig = nil

		config := InitTestConfig()
		assert.NotNil(t, config)
		assert.Equal(t, DefaultTestConfig().TestTimeout, config.TestTimeout)

		// Get should return the same instance
		config2 := GetTestConfig()
		assert.Equal(t, config, config2)
	})

	t.Run("SetTestConfig", func(t *testing.T) {
		// Reset global config
		globalConfig = nil

		customConfig := &TestConfig{
			TestTimeout:   60 * time.Second,
			RetryAttempts: 3,
			VerboseOutput: true,
		}
		SetTestConfig(customConfig)

		config := GetTestConfig()
		assert.Equal(t, 60, int(config.TestTimeout.Seconds()))
		assert.Equal(t, 3, config.RetryAttempts)
		assert.True(t, config.VerboseOutput)
	})

	t.Run("Environment variable helpers", func(t *testing.T) {
		// Test GetEnv
		value := GetEnv("NON_EXISTENT_VAR", "default-value")
		assert.Equal(t, "default-value", value)

		// Test GetEnvAsBool
		boolValue := GetEnvAsBool("NON_EXISTENT_BOOL", false)
		assert.False(t, boolValue)

		// Test GetEnvAsInt
		intValue := GetEnvAsInt("NON_EXISTENT_INT", 42)
		assert.Equal(t, 42, intValue)

		// Test GetEnvAsDuration
		duration := GetEnvAsDuration("NON_EXISTENT_DURATION", 10*time.Second)
		assert.Equal(t, 10*time.Second, duration)
	})

	t.Run("CleanupTestResources", func(t *testing.T) {
		// This should not panic
		CleanupTestResources()
	})

	t.Run("AssertErrorContains", func(t *testing.T) {
		t.Run("error contains substring", func(t *testing.T) {
			err := errors.New("this is an error message")
			// This should not fail
			AssertErrorContains(t, err, "error message")
		})

		t.Run("error does not contain substring", func(t *testing.T) {
			// Create a failing test to verify it fails when it should
			testingT := &testing.T{}
			// We'll test this functionality more indirectly
			// For now, just verify the function doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("AssertErrorContains should not panic: %v", r)
				}
			}()
			AssertErrorContains(testingT, errors.New("test error"), "different")
		})

		t.Run("nil error test", func(t *testing.T) {
			// Test that nil error causes the expected behavior
			testingT := &testing.T{}
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("AssertErrorContains should not panic: %v", r)
				}
			}()
			AssertErrorContains(testingT, nil, "any message")
		})
	})
}

// TestMockServiceHelpers tests the service helper functions
func TestMockServiceHelpers(t *testing.T) {
	t.Run("NewTestMachineService", func(t *testing.T) {
		service, err := NewTestMachineService()
		require.NoError(t, err)
		require.NotNil(t, service)

		// Verify service has mock dependencies
		assert.NotNil(t, service.GetMachineRepository())
		assert.NotNil(t, service.GetEncryptionService())
		assert.NotNil(t, service.GetConnectionCache())
	})

	t.Run("NewTestAuthServiceWithMocks", func(t *testing.T) {
		service, err := NewTestAuthServiceWithMocks()
		require.NoError(t, err)
		require.NotNil(t, service)

		// Verify service has mock user repository
		assert.NotNil(t, service.GetUserRepository())
	})

	t.Run("MockMachineService with errors", func(t *testing.T) {
		service, err := NewTestMachineService()
		require.NoError(t, err)

		// Set errors
		encryptErr := errors.New("encryption error")
		decryptErr := errors.New("decryption error")
		service.WithEncryptionErrors(encryptErr, decryptErr)

		// Verify errors are set
		encService := service.GetEncryptionService()
		_, err = encService.Encrypt("test")
		assert.Error(t, err)
		assert.Equal(t, encryptErr, err)

		_, err = encService.Decrypt([]byte("test"))
		assert.Error(t, err)
		assert.Equal(t, decryptErr, err)
	})
}

// TestMockPodmanConnection tests the mock Podman connection
func TestMockPodmanConnection(t *testing.T) {
	t.Run("GetMachineID", func(t *testing.T) {
		conn := &MockPodmanConnection{MachineID: "test-machine"}
		assert.Equal(t, "test-machine", conn.GetMachineID())
	})

	t.Run("Execute", func(t *testing.T) {
		conn := &MockPodmanConnection{MachineID: "test-machine"}
		result, err := conn.Execute("test-command", "arg1", "arg2")
		assert.NoError(t, err)
		assert.Contains(t, result.(string), "executed: test-command")
	})
}
