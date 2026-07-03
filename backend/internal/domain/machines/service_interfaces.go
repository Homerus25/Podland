package machines

import (
	"context"
)

// ConnectionManagerInterface defines the interface for connection management
// This allows mocking the ConnectionManager in tests
type ConnectionManagerInterface interface {
	GetConnection(ctx context.Context, machineID, userID string) (interface{}, error)
}

// EncryptionServiceInterface defines the interface for encryption services
// This allows mocking the EncryptionService in tests
type EncryptionServiceInterface interface {
	Encrypt(data string) ([]byte, error)
	Decrypt(data []byte) (string, error)
	EncryptSSHKey(privateKey string) ([]byte, string, error)
	DecryptSSHKey(encryptedKey []byte) (string, error)
}