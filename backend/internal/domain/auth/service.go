package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	apperrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	PrivateKeyPath string
	PublicKeyPath  string
	Expiration     time.Duration
	Issuer         string
}

// AuthService handles authentication
type AuthService struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	expiration time.Duration
	issuer     string
	userRepo   models.UserRepositoryInterface
	sessionRepo models.SessionRepositoryInterface
	sessionTTL  time.Duration
}

// NewAuthService creates a new AuthService. sessionTTL is how long a
// refresh session (and its cookie's server-side row) stays redeemable.
func NewAuthService(config JWTConfig, userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, sessionTTL time.Duration) (*AuthService, error) {
	// Load private key
	privateKey, err := loadPrivateKey(config.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	// Load public key
	publicKey, err := loadPublicKey(config.PublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	return &AuthService{
		privateKey: privateKey,
		publicKey:  publicKey,
		expiration: config.Expiration,
		issuer:     config.Issuer,
		userRepo:   userRepo,
		sessionRepo: sessionRepo,
		sessionTTL:  sessionTTL,
	}, nil
}

// Login verifies the given credentials against the user repository and,
// on success, returns a fresh access token, a new opaque refresh token
// (whose SHA-256 hash is persisted as a session row), and the
// authenticated user.
//
// Failure mapping (HTTP-facing, via the structured error responder):
// an unknown username and a wrong password both yield
// ErrInvalidCredentials (401, indistinguishable to the caller), while
// repository and token-signing failures are returned as-is so the
// handler maps them to a generic 500.
//
// Lazy cleanup: login also purges the user's expired session rows —
// there is no background sweeper.
func (s *AuthService) Login(username, password string) (accessToken string, refreshToken string, user *models.User, err error) {
	user, err = s.userRepo.GetByUsername(username)
	if err != nil {
		return "", "", nil, err
	}
	if user == nil {
		return "", "", nil, apperrors.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", "", nil, apperrors.ErrInvalidCredentials
	}

	accessToken, err = s.GenerateToken(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	if err := s.sessionRepo.DeleteExpiredForUser(user.ID); err != nil {
		return "", "", nil, err
	}

	refreshToken, tokenHash, err := generateRefreshToken()
	if err != nil {
		return "", "", nil, err
	}

	// UTC keeps CreatedAt/ExpiresAt comparable against the UTC clock
	// the repository's expiry purge uses, whatever the server's zone.
	now := time.Now().UTC()
	session := &models.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL),
	}
	if err := s.sessionRepo.Create(session); err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, user, nil
}

// Refresh exchanges a presented refresh token for a fresh access JWT and
// a rotated refresh token. The presented token's hash must address a live
// session row; rotation atomically replaces that row with one holding a
// fresh token and a renewed (sliding) expiry, so a token can be redeemed
// exactly once — there is deliberately no non-rotating refresh path.
//
// Failure mapping: an unknown hash — never issued, revoked, or already
// rotated (replayed) — yields ErrRefreshInvalid (401); a found-but-
// expired row is deleted on encounter as lazy cleanup and yields
// ErrSessionExpired (401). A rotation that loses to a concurrent refresh
// presenting the same token also yields ErrRefreshInvalid: the winner
// already consumed the row. Storage failures propagate unchanged (mapped
// to 500 by the handler).
func (s *AuthService) Refresh(refreshToken string) (accessToken, newRefreshToken string, err error) {
	tokenHash := HashRefreshToken(refreshToken)

	session, err := s.sessionRepo.GetByTokenHash(tokenHash)
	if err != nil {
		return "", "", err
	}
	if session == nil {
		return "", "", apperrors.ErrRefreshInvalid
	}

	now := time.Now().UTC()
	if !now.Before(session.ExpiresAt) {
		if err := s.sessionRepo.DeleteByTokenHash(tokenHash); err != nil {
			return "", "", err
		}
		return "", "", apperrors.ErrSessionExpired
	}

	newRefreshToken, newTokenHash, err := generateRefreshToken()
	if err != nil {
		return "", "", err
	}

	accessToken, err = s.GenerateToken(session.UserID)
	if err != nil {
		return "", "", err
	}

	replacement := &models.Session{
		ID:        uuid.New().String(),
		UserID:    session.UserID,
		TokenHash: newTokenHash,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL),
	}
	rotated, err := s.sessionRepo.Rotate(tokenHash, replacement)
	if err != nil {
		return "", "", err
	}
	if !rotated {
		return "", "", apperrors.ErrRefreshInvalid
	}

	return accessToken, newRefreshToken, nil
}

// generateRefreshToken mints a new opaque refresh token: 256 bits of
// crypto-randomness, base64url-encoded. It returns the plaintext token
// (for the cookie — never persisted) and its SHA-256 hex hash (the only
// value stored in the sessions table).
func generateRefreshToken() (token, tokenHash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("failed to read random bytes for refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashRefreshToken(token), nil
}

// HashRefreshToken returns the SHA-256 hex digest of a refresh token —
// the form the sessions table stores and lookups key on.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// loadPrivateKey loads a private key from file
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

// loadPublicKey loads a public key from file
func loadPublicKey(path string) (*rsa.PublicKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return publicKey, nil
}

// GenerateToken generates a JWT token for a user
func (s *AuthService) GenerateToken(userID string) (string, error) {
	// Create claims
	claims := TokenClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Create token with RSA256 signing method
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Sign token with private key
	signedToken, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// ValidateToken validates a JWT token
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (any, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return s.publicKey, nil
	})

	if err != nil {
		// Check for expired token error
		if err.Error() == "token has invalid claims: token is expired" {
			return nil, fmt.Errorf("token expired: %w", err)
		}
		// Check for other validation errors
		if err.Error() == "token contains an invalid number of segments" ||
			err.Error() == "token signature is invalid" ||
			err.Error() == "token is not valid yet" ||
			err.Error() == "token is malformed" {
			return nil, fmt.Errorf("invalid token: %w", err)
		}
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Check if token is valid
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Extract claims
	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Verify issuer
	if claims.Issuer != s.issuer {
		return nil, fmt.Errorf("invalid token issuer")
	}

	return claims, nil
}

// TokenClaims represents JWT claims
type TokenClaims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}
