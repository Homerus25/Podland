package encryption

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustBase64Key encodes raw key material the way production callers do
// before handing it to NewEncryptionService.
func mustBase64Key(t *testing.T, raw []byte) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(raw)
}

// mustService builds an EncryptionService from raw key material, failing
// the test when construction fails.
func mustService(t *testing.T, raw []byte) *EncryptionService {
	t.Helper()
	svc, err := NewEncryptionService(mustBase64Key(t, raw))
	require.NoError(t, err, "service construction should accept %d bytes of key material", len(raw))
	return svc
}

// assertKeysEquivalent pins two pieces of raw key material landing on the
// same internal key: a ciphertext from one service must decrypt under the
// other, in both directions.
func assertKeysEquivalent(t *testing.T, a, b []byte) {
	t.Helper()
	svcA, svcB := mustService(t, a), mustService(t, b)

	got, err := crossDecrypt(t, svcA, svcB, "probe")
	require.NoError(t, err, "second key material should decrypt the first service's ciphertext")
	assert.Equal(t, "probe", got)

	got, err = crossDecrypt(t, svcB, svcA, "probe")
	require.NoError(t, err, "first key material should decrypt the second service's ciphertext")
	assert.Equal(t, "probe", got)
}

// testPrivateKey is representative OpenSSH key material for SSH-key
// roundtrip pins.
const testPrivateKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"

// crossDecrypt encrypts plaintext under one service and decrypts the
// result under another, exposing the decrypt error so pins can require
// success (equivalent keys) or failure (distinct keys).
func crossDecrypt(t *testing.T, from, to *EncryptionService, plaintext string) (string, error) {
	t.Helper()
	ciphertext, err := from.Encrypt(plaintext)
	require.NoError(t, err)
	return to.Decrypt(ciphertext)
}

func testKey32A() []byte { return []byte("0123456789abcdef0123456789abcdef") }
func testKey32B() []byte { return []byte("fedcba9876543210fedcba9876543210") }

// TestEncryptDecryptRoundtrip pins that Encrypt/Decrypt restore arbitrary
// plaintexts, including the empty string and multi-line SSH key material.
func TestEncryptDecryptRoundtrip(t *testing.T) {
	svc := mustService(t, testKey32A())

	plaintexts := []string{
		"",
		"hello podland",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n",
		"unicode \xf0\x9f\x92\xa3 and\ttabs\nand newlines",
	}

	for _, plaintext := range plaintexts {
		ciphertext, err := svc.Encrypt(plaintext)
		require.NoError(t, err, "Encrypt should accept %q", plaintext)
		assert.NotEmpty(t, ciphertext, "Encrypt should produce ciphertext for %q", plaintext)

		got, err := svc.Decrypt(ciphertext)
		require.NoError(t, err, "Decrypt should accept the ciphertext for %q", plaintext)
		assert.Equal(t, plaintext, got)
	}
}

// TestEncrypt_SamePlaintextYieldsDifferentCiphertexts pins the
// per-message nonce: encrypting the same plaintext twice must never
// repeat a ciphertext.
func TestEncrypt_SamePlaintextYieldsDifferentCiphertexts(t *testing.T) {
	svc := mustService(t, testKey32A())

	first, err := svc.Encrypt("same plaintext")
	require.NoError(t, err)
	second, err := svc.Encrypt("same plaintext")
	require.NoError(t, err)

	assert.NotEqual(t, first, second, "two encryptions of the same plaintext must differ (per-message nonce)")
}

// TestEncrypt_DistinctPlaintextsYieldDistinctCiphertexts pins that
// different plaintexts never collide under the same key.
func TestEncrypt_DistinctPlaintextsYieldDistinctCiphertexts(t *testing.T) {
	svc := mustService(t, testKey32A())

	ciphertexts := make(map[string]bool)
	for _, plaintext := range []string{"machine-a-key", "machine-b-key", "machine-c-key"} {
		ciphertext, err := svc.Encrypt(plaintext)
		require.NoError(t, err)
		assert.False(t, ciphertexts[string(ciphertext)],
			"distinct plaintexts must never produce the same ciphertext")
		ciphertexts[string(ciphertext)] = true
	}
}

// TestEncryptSSHKeyDecryptSSHKeyRoundtrip pins the SSH-key pair: the
// returned ciphertext decrypts back to the exact private key material.
func TestEncryptSSHKeyDecryptSSHKeyRoundtrip(t *testing.T) {
	svc := mustService(t, testKey32A())
	privateKey := testPrivateKey
	ciphertext, _, err := svc.EncryptSSHKey(privateKey)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext, "EncryptSSHKey should produce ciphertext")

	decrypted, err := svc.DecryptSSHKey(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, privateKey, decrypted, "DecryptSSHKey must restore the original key material")
}

// TestEncryptSSHKey_FingerprintDerivesDeterministicallyFromCiphertext
// pins the fingerprint contract: the "sha256:<hex>" of the ciphertext
// returned by the same call, so verification stays stable for the stored
// pair. Recomputed here from an independent source (the ciphertext).
func TestEncryptSSHKey_FingerprintDerivesDeterministicallyFromCiphertext(t *testing.T) {
	svc := mustService(t, testKey32A())

	for i := range 3 {
		ciphertext, fingerprint, err := svc.EncryptSSHKey("some private key")
		require.NoError(t, err)

		expected := fmt.Sprintf("sha256:%s", hex.EncodeToString(sum256(ciphertext)))
		assert.Equal(t, expected, fingerprint,
			"call %d: fingerprint must be sha256:hex of the returned ciphertext", i)
	}
}

// TestEncryptSSHKey_FingerprintDistinctForDifferentKeys pins that
// different key materials never share a fingerprint under the same
// service.
func TestEncryptSSHKey_FingerprintDistinctForDifferentKeys(t *testing.T) {
	svc := mustService(t, testKey32A())

	fingerprints := make(map[string]bool)
	for _, privateKey := range []string{"host-alpha key", "host-beta key", "host-gamma key"} {
		_, fingerprint, err := svc.EncryptSSHKey(privateKey)
		require.NoError(t, err)
		assert.NotEmpty(t, fingerprint)
		assert.False(t, fingerprints[fingerprint],
			"different key materials must never produce the same fingerprint")
		fingerprints[fingerprint] = true
	}
}

// TestDecrypt_RejectsTamperedCiphertext pins GCM tamper detection: any
// single flipped byte (nonce, body, or tag) fails decryption instead of
// returning plaintext.
func TestDecrypt_RejectsTamperedCiphertext(t *testing.T) {
	svc := mustService(t, testKey32A())

	ciphertext, err := svc.Encrypt("tamper me")
	require.NoError(t, err)

	for _, position := range []int{0, len(ciphertext) / 2, len(ciphertext) - 1} {
		tampered := make([]byte, len(ciphertext))
		copy(tampered, ciphertext)
		tampered[position] ^= 0xFF

		got, err := svc.Decrypt(tampered)
		assert.Error(t, err, "flipping byte %d must fail decryption", position)
		assert.Empty(t, got, "a tampered ciphertext must never decrypt to plaintext")
	}
}

// TestDecrypt_RejectsCiphertextShorterThanNonce pins the too-short guard
// for inputs that do not even carry a nonce.
func TestDecrypt_RejectsCiphertextShorterThanNonce(t *testing.T) {
	svc := mustService(t, testKey32A())

	for _, data := range [][]byte{nil, []byte("x"), []byte("tiny")} {
		got, err := svc.Decrypt(data)
		assert.Error(t, err, "input of %d bytes must fail decryption", len(data))
		assert.Empty(t, got, "a too-short input must never decrypt to plaintext")
	}
}

// TestDecrypt_WrongKeyFails pins cross-key rejection for Decrypt and the
// DecryptSSHKey delegate: a differently-keyed service must fail, never
// return plaintext.
func TestDecrypt_WrongKeyFails(t *testing.T) {
	svcA := mustService(t, testKey32A())
	svcB := mustService(t, testKey32B())

	got, err := crossDecrypt(t, svcA, svcB, "secret")
	assert.Error(t, err, "a different key must fail to decrypt")
	assert.Empty(t, got, "wrong-key decryption must never return plaintext")

	ciphertext, err := svcA.Encrypt("secret")
	require.NoError(t, err)
	got, err = svcB.DecryptSSHKey(ciphertext)
	assert.Error(t, err, "a different key must fail to decrypt an SSH key")
	assert.Empty(t, got, "wrong-key SSH-key decryption must never return plaintext")
}

// TestNewEncryptionService_RejectsInvalidBase64 pins that a key which is
// not valid base64 is an error, not garbage key material.
func TestNewEncryptionService_RejectsInvalidBase64(t *testing.T) {
	svc, err := NewEncryptionService("not valid base64!!!")

	assert.Error(t, err, "invalid base64 must fail construction")
	assert.Nil(t, svc, "no service should be returned for an invalid key")
}

// TestNewEncryptionService_KeyNormalization pins every constructor
// branch through the exported surface: short keys are SHA-256-extended,
// long keys are truncated to 32 bytes, and a 32-byte key passes through
// untouched. Through the exported surface, pass-through is only
// observable as self-roundtrip plus distinctness from other keys.
func TestNewEncryptionService_KeyNormalization(t *testing.T) {
	shortKey := []byte("short-key")
	longKey := []byte("0123456789abcdef0123456789abcdef0123456789")

	t.Run("short key is SHA-256-extended", func(t *testing.T) {
		assertKeysEquivalent(t, shortKey, sum256(shortKey))
	})

	t.Run("over-long key is truncated to 32 bytes", func(t *testing.T) {
		assertKeysEquivalent(t, longKey, longKey[:32])
	})

	t.Run("32-byte key passes through untouched", func(t *testing.T) {
		svc := mustService(t, testKey32A())

		ciphertext, err := svc.Encrypt("probe")
		require.NoError(t, err)
		got, err := svc.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, "probe", got, "a 32-byte key must roundtrip on its own")
	})

	t.Run("distinct 32-byte keys are not equivalent", func(t *testing.T) {
		got, err := crossDecrypt(t, mustService(t, testKey32A()), mustService(t, testKey32B()), "probe")
		assert.Error(t, err, "two different 32-byte keys must not decrypt each other's ciphertexts")
		assert.Empty(t, got, "cross-key decryption must never return plaintext")
	})
}

// TestGenerateKey_AcceptedByConstructorAndValidation pins that
// GenerateKey emits a fresh 32-byte base64 key the constructor and
// ValidateKey both accept.
func TestGenerateKey_AcceptedByConstructorAndValidation(t *testing.T) {
	key := GenerateKey()

	decoded, err := base64.StdEncoding.DecodeString(key)
	require.NoError(t, err, "GenerateKey must emit valid base64")
	assert.Len(t, decoded, 32, "GenerateKey must emit 32 bytes")

	require.NoError(t, ValidateKey(key), "GenerateKey output must pass ValidateKey")
	_, err = NewEncryptionService(key)
	require.NoError(t, err, "GenerateKey output must be accepted by the constructor")

	other := GenerateKey()
	assert.NotEqual(t, key, other, "two generated keys must differ")
}

// TestValidateKey pins ValidateKey per current behavior: valid base64 of
// at least 16 bytes passes; shorter or non-base64 input fails.
func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{name: "32-byte key is accepted", key: mustBase64Key(t, testKey32A())},
		{name: "16-byte key is accepted (AES-128 minimum)", key: mustBase64Key(t, []byte("0123456789abcdef"))},
		{
			name:    "15-byte key is too short",
			key:     mustBase64Key(t, []byte("0123456789abcde")),
			wantErr: "too short",
		},
		{
			name:    "empty key is too short",
			key:     mustBase64Key(t, nil),
			wantErr: "too short",
		},
		{
			name:    "invalid base64 is rejected",
			key:     "%%% not base64 %%%",
			wantErr: "invalid base64 encoding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if tt.wantErr == "" {
				assert.NoError(t, err, "this key should validate")
				return
			}
			require.Error(t, err, "this key should not validate")
			assert.True(t, strings.Contains(err.Error(), tt.wantErr),
				"error %q should mention %q", err.Error(), tt.wantErr)
		})
	}
}

// sum256 is the raw SHA-256 digest of b, used by the normalization and
// fingerprint pins to state the expected bytes independently.
func sum256(b []byte) []byte {
	digest := sha256.Sum256(b)
	return digest[:]
}
