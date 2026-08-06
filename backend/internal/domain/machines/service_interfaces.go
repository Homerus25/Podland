package machines

// ConnectionManagerInterface defines the interface for connection management
// This allows mocking the ConnectionManager in tests
type ConnectionManagerInterface interface {
	GetConnection(machineID, userID string) (any, error)
	Release(machineID string)
}

// EncryptionServiceInterface defines the interface for encryption services
// This allows mocking the EncryptionService in tests
type EncryptionServiceInterface interface {
	Encrypt(data string) ([]byte, error)
	Decrypt(data []byte) (string, error)
	EncryptSSHKey(privateKey string) ([]byte, string, error)
	DecryptSSHKey(encryptedKey []byte) (string, error)
}
