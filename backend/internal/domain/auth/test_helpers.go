package auth

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/golang-jwt/jwt/v5"
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
// inject repository errors. Session persistence uses a fresh mock
// session repository and the default 30-day TTL.
func NewTestAuthServiceWithUserRepo(userRepo models.UserRepositoryInterface) (*AuthService, error) {
	return NewTestAuthServiceWithSessionRepo(userRepo, testutils.NewMockSessionRepository(), 30*24*time.Hour)
}

// NewTestAuthServiceWithSessionRepo creates a test auth service with
// explicit session dependencies, so session-focused tests can inject a
// pre-seeded session repository, session-repository errors, or a
// non-default TTL. The remembered TTL defaults to the configuration
// default (90 days, issue 379); remembered-session tests that need to
// tell the two TTLs apart use NewTestAuthServiceWithRememberTTL. The
// first-user creator is a loud stub: no auth-level test exercises Setup
// (the first-run contract is pinned at the app wiring seam over the real
// users service), and this helper cannot wire that service without an
// import cycle.
func NewTestAuthServiceWithSessionRepo(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, sessionTTL time.Duration) (*AuthService, error) {
	return NewTestAuthServiceWithRememberTTL(userRepo, sessionRepo, sessionTTL, 90*24*time.Hour)
}

// NewTestAuthServiceWithRememberTTL is NewTestAuthServiceWithSessionRepo
// with the remembered-session TTL supplied by the test, so
// remembered-session tests can pin the two TTLs to distinct values and
// assert which one a given session landed on (issue 379).
func NewTestAuthServiceWithRememberTTL(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, sessionTTL, rememberTTL time.Duration) (*AuthService, error) {
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

	return NewAuthService(config, userRepo, sessionRepo, sessionTTL, rememberTTL, unimplementedFirstUsers{})
}

// NewTestAuthServiceWithFailureBudget is NewTestAuthServiceWithSessionRepo
// with the credential-failure surfaces of the test's choosing wired, so
// the budget tests exercise the check/spend contract. The surfaces are
// constructed by the caller, so the surface names stay at each test's
// own wiring site; a nil surface leaves that endpoint's brake unwired.
// (The auth package cannot import the middleware limiter for real —
// middleware imports auth — so unit tests wire a fake brake and the
// production wiring is pinned at the app and integration seams.)
func NewTestAuthServiceWithFailureBudget(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, login, change *failurebudget.Surface) (*AuthService, error) {
	service, err := NewTestAuthServiceWithSessionRepo(userRepo, sessionRepo, 30*24*time.Hour)
	if err != nil {
		return nil, err
	}
	return service.WithLoginBudget(login).WithChangeBudget(change), nil
}

// unimplementedFirstUsers is the test helpers' stand-in FirstUserCreator.
// Any accidental Setup call through a helper-built service fails loudly
// instead of half-working.
type unimplementedFirstUsers struct{}

func (unimplementedFirstUsers) CreateFirstAdmin(username, password string) (*models.User, error) {
	return nil, fmt.Errorf("auth test service: Setup needs a real FirstUserCreator; wire users.NewUserService as app.go does")
}

// Helper function to write string to file
func writeStringToFile(filename, content string) error {
	return os.WriteFile(filename, []byte(content), 0600)
}

// Helper function to cleanup file
func cleanupFile(filename string) {
	os.Remove(filename)
}

// CreateExpiredTestToken mints a genuinely expired RS256 JWT: signed
// with the service's own key and issuer, with an expiry an hour in the
// past, so ValidateToken takes the real expired path and answers
// ErrTokenExpired (and the JWT middleware the TOKEN_EXPIRED envelope).
func CreateExpiredTestToken(service *AuthService, userID string) (string, error) {
	claims := TokenClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    service.issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(service.privateKey)
}

// CreateValidTestToken creates a valid JWT token for testing
func CreateValidTestToken(service *AuthService, userID string) (string, error) {
	return service.GenerateToken(userID)
}
