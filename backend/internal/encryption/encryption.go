package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptionService handles encryption and decryption of sensitive data
type EncryptionService struct {
	key []byte
}

// NewEncryptionService creates a new EncryptionService with the given key
// The key should be a 32-byte base64-encoded string for AES-256
func NewEncryptionService(key string) (*EncryptionService, error) {
	// Decode the base64 key
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key: %w", err)
	}

	// Ensure the key is the correct size (32 bytes for AES-256)
	if len(decodedKey) != 32 {
		// If the key is too short, extend it with SHA256
		if len(decodedKey) < 32 {
			hash := sha256.Sum256(decodedKey)
			decodedKey = hash[:]
		} else {
			// If the key is too long, truncate it
			decodedKey = decodedKey[:32]
		}
	}

	return &EncryptionService{key: decodedKey}, nil
}

// Encrypt encrypts data using AES-256-GCM
func (s *EncryptionService) Encrypt(data string) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	plaintext := []byte(data)
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM
func (s *EncryptionService) Decrypt(data []byte) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// EncryptSSHKey encrypts an SSH private key and returns a simple hash as fingerprint
func (s *EncryptionService) EncryptSSHKey(privateKey string) ([]byte, string, error) {
	// Encrypt the private key
	encryptedKey, err := s.Encrypt(privateKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encrypt SSH key: %w", err)
	}

	// Generate fingerprint as hash of encrypted data
	fingerprint := fmt.Sprintf("sha256:%x", sha256.Sum256(encryptedKey))

	return encryptedKey, fingerprint, nil
}

// DecryptSSHKey decrypts an SSH private key
func (s *EncryptionService) DecryptSSHKey(encryptedKey []byte) (string, error) {
	return s.Decrypt(encryptedKey)
}

// ValidateKey checks if a key is valid for AES-256
func ValidateKey(key string) error {
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("invalid base64 encoding")
	}

	if len(decodedKey) < 16 {
		return fmt.Errorf("key too short, minimum 16 bytes required for AES-128")
	}

	return nil
}

// GenerateKey generates a new 32-byte key and returns it as base64
func GenerateKey() string {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}
