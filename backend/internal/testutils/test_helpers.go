package testutils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/Homerus25/podland/backend/internal/models"
)

// GenerateTestUser creates a test user with random ID
func GenerateTestUser() *models.User {
	return &models.User{
		ID:           uuid.New().String(),
		Username:     fmt.Sprintf("testuser-%s", uuid.New().String()[:8]),
		PasswordHash: "test-password-hash",
		CreatedAt:    time.Now(),
	}
}

// GenerateTestMachine creates a test machine with random ID
func GenerateTestMachine() *models.Machine {
	return &models.Machine{
		ID:          uuid.New().String(),
		Name:        fmt.Sprintf("test-machine-%s", uuid.New().String()[:8]),
		OwnerID:     uuid.New().String(),
		Hostname:    "example.com",
		SSHUsername: "root",
		Port:        22,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// GenerateTestMachineWithOwner creates a test machine with specified owner ID
func GenerateTestMachineWithOwner(ownerID string) *models.Machine {
	return &models.Machine{
		ID:          uuid.New().String(),
		Name:        fmt.Sprintf("test-machine-%s", uuid.New().String()[:8]),
		OwnerID:     ownerID,
		Hostname:    "example.com",
		SSHUsername: "root",
		Port:        22,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// GenerateValidPEMKey creates a valid RSA private key in PEM format
func GenerateValidPEMKey() string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate test RSA key: %v", err))
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	return string(pem.EncodeToMemory(privateKeyPEM))
}

// GenerateInvalidPEMKey creates an invalid PEM key for testing
func GenerateInvalidPEMKey() string {
	return "invalid-key-data"
}

// GenerateValidOpenSSHKey creates a valid OpenSSH private key in PEM format
func GenerateValidOpenSSHKey() string {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate test RSA key: %v", err))
	}

	// Convert to OpenSSH format
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal private key: %v", err))
	}

	privateKeyPEM := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	return string(pem.EncodeToMemory(privateKeyPEM))
}

// GenerateValidECKey creates a valid EC private key in PEM format
func GenerateValidECKey() string {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("failed to generate test EC key: %v", err))
	}

	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal EC private key: %v", err))
	}

	privateKeyPEM := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	return string(pem.EncodeToMemory(privateKeyPEM))
}

// CreateTestJWTKeys creates test JWT keys for signing and validation
func CreateTestJWTKeys() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %v", err)
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}

	return string(pem.EncodeToMemory(privateKeyPEM)), string(pem.EncodeToMemory(publicKeyPEM)), nil
}

// CleanupTestResources cleans up any test resources
func CleanupTestResources() {
	// Currently a no-op, but can be extended for actual resource cleanup
}

// NewTestMachineService creates a test machine service with mock dependencies
func NewTestMachineService() (*MockMachineService, error) {
	mockRepo := NewMockMachineRepository()
	mockEncryption := NewMockEncryptionService()
	mockCache := NewMockConnectionCache()

	return &MockMachineService{
		mockMachineRepo:     mockRepo,
		mockEncryption:      mockEncryption,
		mockConnectionCache: mockCache,
	}, nil
}

// NewTestAuthServiceWithMocks creates a test auth service with mock dependencies
func NewTestAuthServiceWithMocks() (*MockAuthService, error) {
	mockRepo := NewMockUserRepository()

	return &MockAuthService{
		mockUserRepo: mockRepo,
	}, nil
}

// MockMachineService is a mock machine service for testing
type MockMachineService struct {
	mockMachineRepo     *MockMachineRepository
	mockEncryption      *MockEncryptionService
	mockConnectionCache *MockConnectionCache
	errors              map[string]error
}

// MockAuthService is a mock auth service for testing
type MockAuthService struct {
	mockUserRepo *MockUserRepository
	errors       map[string]error
}

// WithRepositoryErrors sets errors for the mock machine repository
func (s *MockMachineService) WithRepositoryErrors(errors map[string]error) *MockMachineService {
	s.errors = errors
	return s
}

// WithEncryptionErrors sets errors for the mock encryption service
func (s *MockMachineService) WithEncryptionErrors(encryptErr, decryptErr error) *MockMachineService {
	s.mockEncryption.encryptError = encryptErr
	s.mockEncryption.decryptError = decryptErr
	return s
}

// GetMachineRepository returns the mock machine repository
func (s *MockMachineService) GetMachineRepository() *MockMachineRepository {
	return s.mockMachineRepo
}

// GetEncryptionService returns the mock encryption service
func (s *MockMachineService) GetEncryptionService() *MockEncryptionService {
	return s.mockEncryption
}

// GetConnectionCache returns the mock connection cache
func (s *MockMachineService) GetConnectionCache() *MockConnectionCache {
	return s.mockConnectionCache
}

// GetUserRepository returns the mock user repository
func (s *MockAuthService) GetUserRepository() *MockUserRepository {
	return s.mockUserRepo
}

// AssertErrorContains checks if an error contains a specific substring
func AssertErrorContains(t testing.TB, err error, expectedSubstring string) {
	if err == nil {
		t.Helper()
		t.Errorf("Expected error containing '%s', got nil", expectedSubstring)
		// Call Fail to mark the test as failed
		if f, ok := t.(interface{ Fail() }); ok {
			f.Fail()
		}
		return
	}
	if !containsSubstring(err.Error(), expectedSubstring) {
		t.Helper()
		t.Errorf("Expected error to contain '%s', got: %v", expectedSubstring, err)
		// Call Fail to mark the test as failed
		if f, ok := t.(interface{ Fail() }); ok {
			f.Fail()
		}
	}
}

// Helper function to check if string contains substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s[1:], substr)))
}
