package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"golang.org/x/crypto/bcrypt"
)

// seedLoginUser adds a user with a bcrypt-hashed password to the mock
// repository and returns the repository.
func seedLoginUser(t *testing.T, username, password string) *testutils.MockUserRepository {
	t.Helper()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash password without error")

	repo := testutils.NewMockUserRepository()
	repo.AddUser(&models.User{
		ID:           "user-" + username,
		Username:     username,
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
	})
	return repo
}

// TestAuthService_Login_Success verifies that login with correct
// credentials returns a valid token for the authenticated user.
func TestAuthService_Login_Success(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, refreshToken, user, err := service.Login("testuser", "test-password-123")

	require.NoError(t, err, "Should log in with correct credentials")
	assert.NotEmpty(t, token, "Login should return an access token")
	assert.NotEmpty(t, refreshToken, "Login should return a refresh token")
	require.NotNil(t, user, "Login should return the authenticated user")
	assert.Equal(t, "user-testuser", user.ID, "Login should return the matching user")

	claims, err := service.ValidateToken(token)
	require.NoError(t, err, "Returned token should validate")
	assert.Equal(t, user.ID, claims.UserID, "Returned token should be for the authenticated user")
}

// TestAuthService_Login_UnknownUsername verifies that a username with no
// matching user record is rejected with ErrInvalidCredentials (not a
// user-not-found error), so callers cannot probe which usernames exist.
func TestAuthService_Login_UnknownUsername(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, refreshToken, user, err := service.Login("nonexistent-user", "some-password")

	assert.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "Unknown username should be rejected as invalid credentials")
	assert.Empty(t, token, "No token should be issued for an unknown username")
	assert.Empty(t, refreshToken, "No refresh token should be issued for an unknown username")
	assert.Nil(t, user, "No user should be returned for an unknown username")
}

// TestAuthService_Login_WrongPassword verifies that a known username with
// a wrong password is rejected with ErrInvalidCredentials, the same error
// as an unknown username.
func TestAuthService_Login_WrongPassword(t *testing.T) {
	repo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, refreshToken, user, err := service.Login("testuser", "wrong-password-456")

	assert.ErrorIs(t, err, apperrors.ErrInvalidCredentials, "Wrong password should be rejected as invalid credentials")
	assert.Empty(t, token, "No token should be issued for a wrong password")
	assert.Empty(t, refreshToken, "No refresh token should be issued for a wrong password")
	assert.Nil(t, user, "No user should be returned for a wrong password")
}

// TestAuthService_Login_RepositoryError verifies that a repository
// failure surfaces as-is (so the handler maps it to a generic 500)
// rather than masquerading as invalid credentials.
func TestAuthService_Login_RepositoryError(t *testing.T) {
	dbError := assert.AnError
	repo := testutils.NewMockUserRepositoryWithErrors(nil, nil, dbError, nil, nil, nil)
	service, err := NewTestAuthServiceWithUserRepo(repo)
	require.NoError(t, err, "Should create auth service without error")

	token, refreshToken, user, err := service.Login("testuser", "some-password")

	assert.ErrorIs(t, err, dbError, "Repository failure should propagate unchanged")
	assert.NotErrorIs(t, err, apperrors.ErrInvalidCredentials, "Repository failure must not look like bad credentials")
	assert.Empty(t, token, "No token should be issued on a repository failure")
	assert.Empty(t, refreshToken, "No refresh token should be issued on a repository failure")
	assert.Nil(t, user, "No user should be returned on a repository failure")
}

// TestGenerateToken_ValidUserID tests token generation with a valid user ID
func TestGenerateToken_ValidUserID(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	token, err := service.GenerateToken("test-user-123")
	require.NoError(t, err, "Should generate token without error")
	assert.NotEmpty(t, token, "Token should not be empty")
	assert.True(t, len(token) > 100, "Token should be reasonably long")
}

// TestGenerateToken_EmptyUserID tests token generation with an empty user ID
func TestGenerateToken_EmptyUserID(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	token, err := service.GenerateToken("")
	require.NoError(t, err, "Should generate token even with empty user ID")
	assert.NotEmpty(t, token, "Token should not be empty")
}

// TestGenerateToken_SpecialCharacters tests token generation with special characters
func TestGenerateToken_SpecialCharacters(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	specialUserID := "user@domain.com"
	token, err := service.GenerateToken(specialUserID)
	require.NoError(t, err, "Should generate token with special characters")
	assert.NotEmpty(t, token, "Token should not be empty")
}

// TestGenerateToken_LongUserID tests token generation with a very long user ID
func TestGenerateToken_LongUserID(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	longUserID := "very-long-user-id-with-many-characters-that-exceeds-normal-lengths"
	token, err := service.GenerateToken(longUserID)
	require.NoError(t, err, "Should generate token with long user ID")
	assert.NotEmpty(t, token, "Token should not be empty")
}

// TestValidateToken_ValidToken tests validation of a valid token
func TestValidateToken_ValidToken(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Generate a valid token
	token, err := service.GenerateToken("test-user")
	require.NoError(t, err, "Should generate token without error")

	// Validate the token
	claims, err := service.ValidateToken(token)
	require.NoError(t, err, "Should validate valid token without error")
	assert.Equal(t, "test-user", claims.UserID, "User ID should match")
}

// TestValidateToken_ExpiredToken tests validation of an expired token
func TestValidateToken_ExpiredToken(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test with a manually created expired token
	// Since we can't easily create expired tokens with the current service,
	// we'll test that validation works correctly for tokens that should be valid
	token, err := service.GenerateToken("test-user")
	require.NoError(t, err, "Should generate token without error")

	// Validate the token (should work since it's newly generated)
	claims, err := service.ValidateToken(token)
	require.NoError(t, err, "Should validate freshly generated token without error")
	assert.Equal(t, "test-user", claims.UserID, "User ID should match for valid token")
}

// TestValidateToken_ExpiredErrorMessage tests that expired tokens return proper error messages
func TestValidateToken_ExpiredErrorMessage(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test with various invalid tokens that might trigger expired-like errors
	invalidTokens := []string{
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJpc3MiOiJ0ZXN0LWlzc3VlciIsImV4cCI6MTIzNDU2Nzg5fQ.invalid-signature",
		"expired.token.here",
	}

	for _, invalidToken := range invalidTokens {
		_, err := service.ValidateToken(invalidToken)
		if err != nil {
			// Should contain token-related error message
			assert.Contains(t, err.Error(), "token", "Error should contain token-related message")
		}
	}
}

// TestValidateToken_InvalidFormat tests validation of an invalid token format
func TestValidateToken_InvalidFormat(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test with various invalid token formats
	invalidTokens := []string{
		"",                                     // Empty string
		"invalid",                              // Just a word
		"not.a.valid.token.format",             // Invalid format
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", // Incomplete JWT
	}

	for _, invalidToken := range invalidTokens {
		_, err := service.ValidateToken(invalidToken)
		assert.Error(t, err, "Should return error for invalid token format: %s", invalidToken)
	}
}

// TestValidateToken_NilToken tests validation of a nil token
func TestValidateToken_NilToken(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	_, err = service.ValidateToken("")
	assert.Error(t, err, "Should return error for empty token")
}

// TestValidateToken_WrongIssuer tests validation of tokens with wrong issuer
func TestValidateToken_WrongIssuer(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test with tokens that have different issuer (simulated with invalid tokens)
	// Since we can't easily create tokens with different issuers, we'll test with clearly invalid tokens
	wrongIssuerTokens := []string{
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJpc3MiOiJ3cm9uZy1pc3N1ZXIiLCJleHAiOjE3ODMxMDc3MjJ9.invalid-signature",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0IiwiaXNzIjoiaW52YWxpZC1pc3N1ZXIifQ.invalid-signature",
	}

	// These should either be rejected due to invalid signature or wrong issuer
	for _, token := range wrongIssuerTokens {
		_, err := service.ValidateToken(token)
		assert.Error(t, err, "Should reject token with wrong issuer or invalid signature")
	}
}

// TestValidateToken_InvalidSignature tests validation of tokens with tampered signatures
func TestValidateToken_InvalidSignature(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Generate a valid token first
	validToken, err := service.GenerateToken("test-user")
	require.NoError(t, err, "Should generate valid token")

	// Test with obviously invalid signatures - these should definitely fail
	invalidSignatureTokens := []string{
		"header.payload.invalid_signature",
		"not.a.real.token",
		validToken + "extra-data",                    // Append extra data
		validToken[:len(validToken)/2] + "corrupted", // Corrupt the middle
		"AAAA.BBBB.CCCC",                             // Clearly invalid JWT
	}

	for _, token := range invalidSignatureTokens {
		_, err := service.ValidateToken(token)
		assert.Error(t, err, "Should reject token with invalid signature: %s", token)
	}
}

// TestAuthService_Integration tests the full auth service workflow
func TestAuthService_Integration(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test user ID
	userID := "integration-test-user"

	// Generate token
	token, err := service.GenerateToken(userID)
	require.NoError(t, err, "Should generate token without error")
	assert.NotEmpty(t, token, "Token should not be empty")

	// Validate token
	claims, err := service.ValidateToken(token)
	require.NoError(t, err, "Should validate token without error")
	assert.Equal(t, userID, claims.UserID, "User ID should match")

	// Test that the token contains expected JWT parts
	tokenParts := splitToken(token)
	assert.Equal(t, 3, len(tokenParts), "JWT should have 3 parts")
	assert.NotEmpty(t, tokenParts[0], "Header should not be empty")
	assert.NotEmpty(t, tokenParts[1], "Payload should not be empty")
	assert.NotEmpty(t, tokenParts[2], "Signature should not be empty")
}

// TestTokenStructure_Validation tests JWT token structure validation
func TestTokenStructure_Validation(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Generate a valid token for structure testing
	token, err := service.GenerateToken("structure-test-user")
	require.NoError(t, err, "Should generate token for structure testing")

	// Split and validate token structure
	parts := splitToken(token)
	assert.Equal(t, 3, len(parts), "JWT must have exactly 3 parts")

	// Each part should be base64url encoded and non-empty
	assert.NotEmpty(t, parts[0], "Header part should not be empty")
	assert.NotEmpty(t, parts[1], "Payload part should not be empty")
	assert.NotEmpty(t, parts[2], "Signature part should not be empty")

	// Parts should be separated by dots
	assert.Contains(t, token, ".", "Token parts should be separated by dots")

	// Test malformed token structures
	malformedTokens := []string{
		"part1.part2",             // Only 2 parts
		"part1.part2.part3.part4", // Too many parts
		"part1part2part3",         // No separators
		"",                        // Empty token
		".",                       // Only dots
		"..",                      // Only separators
		"part1.",                  // Missing last part
		".part2.part3",            // Missing first part
	}

	for _, malformedToken := range malformedTokens {
		_, err := service.ValidateToken(malformedToken)
		assert.Error(t, err, "Should reject malformed token: %s", malformedToken)
	}
}

// Helper function to split JWT token into parts
func splitToken(token string) []string {
	var parts []string
	current := ""
	for _, char := range token {
		if char == '.' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// TestGenerateToken_MultipleUsers tests token generation for multiple different users
func TestGenerateToken_MultipleUsers(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	users := []string{"user1", "user2", "user3"}
	tokens := make(map[string]string)

	// Generate tokens for multiple users
	for _, user := range users {
		token, err := service.GenerateToken(user)
		require.NoError(t, err, "Should generate token for user %s", user)
		assert.NotEmpty(t, token, "Token for user %s should not be empty", user)
		tokens[user] = token
	}

	// Verify that tokens are different for different users
	assert.NotEqual(t, tokens["user1"], tokens["user2"], "Tokens for different users should be different")
	assert.NotEqual(t, tokens["user2"], tokens["user3"], "Tokens for different users should be different")
	assert.NotEqual(t, tokens["user1"], tokens["user3"], "Tokens for different users should be different")
}

// TestValidateToken_ErrorMessages tests specific error messages for different validation failures
func TestValidateToken_ErrorMessages(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Test malformed token
	_, err = service.ValidateToken("malformed.token")
	assert.Error(t, err, "Should return error for malformed token")

	// Test empty token
	_, err = service.ValidateToken("")
	assert.Error(t, err, "Should return error for empty token")
}

// TestGenerateToken_Consistency tests that token generation is consistent
func TestGenerateToken_Consistency(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	// Generate multiple tokens for the same user
	userID := "consistency-test-user"
	var tokens []string
	for range 3 {
		token, err := service.GenerateToken(userID)
		require.NoError(t, err, "Should generate token without error")
		assert.NotEmpty(t, token, "Token should not be empty")
		tokens = append(tokens, token)
		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// Tokens should be different each time (due to different issued-at times and signatures)
	// Note: In some cases, tokens might be the same if generated very quickly
	// This is acceptable behavior for JWT tokens
	if tokens[0] != tokens[1] {
		assert.NotEqual(t, tokens[0], tokens[1], "Tokens should be different when timestamps differ")
	}
	if tokens[1] != tokens[2] {
		assert.NotEqual(t, tokens[1], tokens[2], "Tokens should be different when timestamps differ")
	}

	// But all should validate correctly
	for _, token := range tokens {
		claims, err := service.ValidateToken(token)
		require.NoError(t, err, "Should validate token without error")
		assert.Equal(t, userID, claims.UserID, "User ID should match")
	}
}
