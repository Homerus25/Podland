package testutils

import (
	"context"
	"fmt"
)

// MockEncryptionService implements a mock encryption service for testing
type MockEncryptionService struct {
	encryptError     error
	decryptError     error
	encryptFunc      func(string) ([]byte, error)
	decryptFunc      func([]byte) (string, error)
	encryptSSHFunc   func(string) ([]byte, string, error)
	decryptSSHFunc   func([]byte) (string, error)
}

func NewMockEncryptionService() *MockEncryptionService {
	return &MockEncryptionService{}
}

func NewMockEncryptionServiceWithErrors(encryptErr, decryptErr error) *MockEncryptionService {
	return &MockEncryptionService{
		encryptError: encryptErr,
		decryptError: decryptErr,
	}
}

// NewMockEncryptionServiceWithCustomFunctions allows custom encryption/decryption logic
func NewMockEncryptionServiceWithCustomFunctions(
	encryptFunc func(string) ([]byte, error),
	decryptFunc func([]byte) (string, error),
	encryptSSHFunc func(string) ([]byte, string, error),
	decryptSSHFunc func([]byte) (string, error),
) *MockEncryptionService {
	return &MockEncryptionService{
		encryptFunc:    encryptFunc,
		decryptFunc:    decryptFunc,
		encryptSSHFunc: encryptSSHFunc,
		decryptSSHFunc: decryptSSHFunc,
	}
}

func (m *MockEncryptionService) Encrypt(data string) ([]byte, error) {
	if m.encryptError != nil {
		return nil, m.encryptError
	}
	if m.encryptFunc != nil {
		return m.encryptFunc(data)
	}
	// Simple mock encryption - just convert to bytes with prefix
	return []byte(fmt.Sprintf("encrypted:%s", data)), nil
}

func (m *MockEncryptionService) Decrypt(data []byte) (string, error) {
	if m.decryptError != nil {
		return "", m.decryptError
	}
	if m.decryptFunc != nil {
		return m.decryptFunc(data)
	}
	// Simple mock decryption - remove the prefix
	dataStr := string(data)
	if len(dataStr) > len("encrypted:") && dataStr[:len("encrypted:")] == "encrypted:" {
		return dataStr[len("encrypted:"):], nil
	}
	return dataStr, nil
}

func (m *MockEncryptionService) EncryptSSHKey(privateKey string) ([]byte, string, error) {
	if m.encryptError != nil {
		return nil, "", m.encryptError
	}
	if m.encryptSSHFunc != nil {
		return m.encryptSSHFunc(privateKey)
	}
	// Simple mock SSH encryption
	encrypted := []byte(fmt.Sprintf("ssh-encrypted:%s", privateKey))
	fingerprint := fmt.Sprintf("sha256:%x", []byte(privateKey))
	return encrypted, fingerprint, nil
}

func (m *MockEncryptionService) DecryptSSHKey(encryptedKey []byte) (string, error) {
	if m.decryptError != nil {
		return "", m.decryptError
	}
	if m.decryptSSHFunc != nil {
		return m.decryptSSHFunc(encryptedKey)
	}
	// Simple mock SSH decryption
	dataStr := string(encryptedKey)
	if len(dataStr) > len("ssh-encrypted:") && dataStr[:len("ssh-encrypted:")] == "ssh-encrypted:" {
		return dataStr[len("ssh-encrypted:"):], nil
	}
	return dataStr, nil
}

// MockPodmanConnectionService implements a mock Podman connection service
type MockPodmanConnectionService struct {
	connectError error
	closeError   error
}

func NewMockPodmanConnectionService() *MockPodmanConnectionService {
	return &MockPodmanConnectionService{}
}

func NewMockPodmanConnectionServiceWithErrors(connectErr, closeErr error) *MockPodmanConnectionService {
	return &MockPodmanConnectionService{
		connectError: connectErr,
		closeError:   closeErr,
	}
}

func (m *MockPodmanConnectionService) Connect(ctx context.Context, machineID string, sshConfig interface{}) (interface{}, error) {
	if m.connectError != nil {
		return nil, m.connectError
	}
	// Return a mock connection
	return &MockPodmanConnection{MachineID: machineID}, nil
}

func (m *MockPodmanConnectionService) Close(ctx context.Context, connection interface{}) error {
	if m.closeError != nil {
		return m.closeError
	}
	return nil
}

// MockPodmanConnection represents a mock Podman connection
type MockPodmanConnection struct {
	MachineID string
}

func (m *MockPodmanConnection) GetMachineID() string {
	return m.MachineID
}

func (m *MockPodmanConnection) Execute(command string, args ...interface{}) (interface{}, error) {
	// Simple mock execution - just return the command
	return fmt.Sprintf("executed: %s %v", command, args), nil
}

// MockConnectionManager implements a mock connection manager for testing
type MockConnectionManager struct {
	getConnectionErr error
}

func NewMockConnectionManager() *MockConnectionManager {
	return &MockConnectionManager{}
}

func NewMockConnectionManagerWithError(getConnectionErr error) *MockConnectionManager {
	return &MockConnectionManager{
		getConnectionErr: getConnectionErr,
	}
}

func (m *MockConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (interface{}, error) {
	if m.getConnectionErr != nil {
		return nil, m.getConnectionErr
	}
	return &MockPodmanConnection{MachineID: machineID}, nil
}

// Helper function to create a test machine service with all mock dependencies
// Note: This returns individual mock components that can be used to create a MachineService
// The MachineService itself is created by the caller to maintain proper package boundaries
func NewTestMachineServiceComponents() (*MockMachineRepository, *MockMachineAccessRepository, *MockConnectionManager, *MockEncryptionService) {
	return NewMockMachineRepository(), NewMockMachineAccessRepository(), NewMockConnectionManager(), NewMockEncryptionService()
}