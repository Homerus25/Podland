package auth

import (
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yourorg/podland/backend/internal/models"
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
	privateKey     *rsa.PrivateKey
	publicKey      *rsa.PublicKey
	expiration     time.Duration
	issuer         string
	userRepo       models.UserRepositoryInterface
}

// NewAuthService creates a new AuthService
func NewAuthService(config JWTConfig, userRepo models.UserRepositoryInterface) (*AuthService, error) {
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
		privateKey:     privateKey,
		publicKey:      publicKey,
		expiration:     config.Expiration,
		issuer:         config.Issuer,
		userRepo:       userRepo,
	}, nil
}

// loadPrivateKey loads a private key from file
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	keyBytes, err := ioutil.ReadFile(path)
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
	keyBytes, err := ioutil.ReadFile(path)
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
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
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
