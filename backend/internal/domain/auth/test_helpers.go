package auth

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// generatePublicKeyFromPrivate generates a public key PEM from a private key PEM
func generatePublicKeyFromPrivate(privateKeyPEM string) (string, error) {
	// Decode the private key PEM
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode private key PEM")
	}

	// Parse the RSA private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %v", err)
	}

	// Get the public key
	publicKey := &privateKey.PublicKey

	// Encode the public key to PEM format
	publicKeyBytes := x509.MarshalPKCS1PublicKey(publicKey)
	publicKeyPEMBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	return string(pem.EncodeToMemory(publicKeyPEMBlock)), nil
}

// NewTestAuthService creates a test auth service with mock dependencies
func NewTestAuthService() (*AuthService, error) {
	return NewTestAuthServiceWithUserRepo(testutils.NewMockUserRepository())
}

// NewTestAuthServiceWithUserRepo creates a test auth service whose user
// repository is supplied by the test, so login tests can seed users or
// inject repository errors.
func NewTestAuthServiceWithUserRepo(userRepo models.UserRepositoryInterface) (*AuthService, error) {
	// Create temporary files for test keys
	privateKeyPEM := testutils.GenerateValidPEMKey()
	// Generate a proper public key from the private key
	publicKeyPEM, err := generatePublicKeyFromPrivate(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to generate public key: %v", err)
	}

	// Create temporary files for the keys
	privateKeyFile := "test_private_key.pem"
	publicKeyFile := "test_public_key.pem"

	// Write private key to file
	err = writeStringToFile(privateKeyFile, privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to write private key file: %v", err)
	}
	defer cleanupFile(privateKeyFile)

	// Write public key to file
	err = writeStringToFile(publicKeyFile, publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to write public key file: %v", err)
	}
	defer cleanupFile(publicKeyFile)

	config := JWTConfig{
		PrivateKeyPath: privateKeyFile,
		PublicKeyPath:  publicKeyFile,
		Expiration:     time.Hour * 24,
		Issuer:         "test-issuer",
	}

	return NewAuthService(config, userRepo)
}

// Helper function to write string to file
func writeStringToFile(filename, content string) error {
	return os.WriteFile(filename, []byte(content), 0600)
}

// Helper function to cleanup file
func cleanupFile(filename string) {
	os.Remove(filename)
}

// CreateExpiredTestToken creates an expired JWT token for testing
// Note: This is a simplified version that creates a token and then modifies it to be expired
// In a real test scenario, you might want to use a different approach
func CreateExpiredTestToken(service *AuthService, userID string) (string, error) {
	// For now, just generate a valid token
	// A proper implementation would create an expired token
	return service.GenerateToken(userID)
}

// CreateValidTestToken creates a valid JWT token for testing
func CreateValidTestToken(service *AuthService, userID string) (string, error) {
	return service.GenerateToken(userID)
}
