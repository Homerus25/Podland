package testutils

import (
	"context"
	"fmt"
)

// MockEncryptionService implements a mock encryption service for testing
type MockEncryptionService struct {
	encryptError error
	decryptError error
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

func (m *MockEncryptionService) Encrypt(ctx context.Context, data string) (string, error) {
	if m.encryptError != nil {
		return "", m.encryptError
	}
	// Simple mock encryption - just add a prefix
	return fmt.Sprintf("encrypted:%s", data), nil
}

func (m *MockEncryptionService) Decrypt(ctx context.Context, encryptedData string) (string, error) {
	if m.decryptError != nil {
		return "", m.decryptError
	}
	// Simple mock decryption - remove the prefix
	if len(encryptedData) > len("encrypted:") && encryptedData[:len("encrypted:")] == "encrypted:" {
		return encryptedData[len("encrypted:"):], nil
	}
	return encryptedData, nil
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
	return &MockPodmanConnection{machineID: machineID}, nil
}

func (m *MockPodmanConnectionService) Close(ctx context.Context, connection interface{}) error {
	if m.closeError != nil {
		return m.closeError
	}
	return nil
}

// MockPodmanConnection represents a mock Podman connection
type MockPodmanConnection struct {
	machineID string
}

func (m *MockPodmanConnection) GetMachineID() string {
	return m.machineID
}

func (m *MockPodmanConnection) Execute(command string, args ...interface{}) (interface{}, error) {
	// Simple mock execution - just return the command
	return fmt.Sprintf("executed: %s %v", command, args), nil
}