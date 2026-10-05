package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	PrivateKeyPath string
	PublicKeyPath  string
	Expiration     time.Duration
	Issuer         string
}

// FirstUserCreator is the seam Setup needs: the shared user-creation
// service's first-admin path (validation + hash + the atomic first-user
// guard). Declared here consumer-side — auth never imports the users
// package, and with the failure budget's move to internal/failurebudget
// (issue 236) this is the credential surfaces' one remaining
// consumer-side interface: it has exactly one consumer (internal/app
// wires users.UserService in), so the third-copy rule has not crossed.
// The returned user is the freshly created admin, or an AppError: the
// creation path's validation codes, or ErrSetupCompleted when the
// instance already has a user.
type FirstUserCreator interface {
	CreateFirstAdmin(username, password string) (*models.User, error)
}

// AuthService handles authentication
type AuthService struct {
	privateKey   *rsa.PrivateKey
	publicKey    *rsa.PublicKey
	expiration   time.Duration
	issuer       string
	userRepo     models.UserRepositoryInterface
	sessionRepo  models.SessionRepositoryInterface
	sessionTTL   time.Duration
	firstUsers   FirstUserCreator
	loginBudget  *failurebudget.Surface
	changeBudget *failurebudget.Surface
}

// NewAuthService creates a new AuthService. sessionTTL is how long a
// refresh session (and its cookie's server-side row) stays redeemable.
// firstUsers backs Setup; Login, Refresh, Logout, and ChangePassword
// never touch it. The credential-failure budgets are optional
// dependencies wired via WithLoginBudget / WithChangeBudget after
// construction (issue 236): unwired, every check reports budget and
// every failure records nothing.
func NewAuthService(config JWTConfig, userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, sessionTTL time.Duration, firstUsers FirstUserCreator) (*AuthService, error) {
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
		privateKey:  privateKey,
		publicKey:   publicKey,
		expiration:  config.Expiration,
		issuer:      config.Issuer,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		sessionTTL:  sessionTTL,
		firstUsers:  firstUsers,
	}, nil
}

// WithLoginBudget wires the login endpoint's credential-failure budget
// (issue 236): the failurebudget.Surface whose Check Login consults
// before any verification and whose Spend a failed verification pays.
// The fluent shape mirrors the volumes handler's WithMaxUploadBytes:
// an optional dependency layered onto the constructor. Unwired (a nil
// surface), the brake is off — every check reports budget.
func (s *AuthService) WithLoginBudget(budget *failurebudget.Surface) *AuthService {
	s.loginBudget = budget
	return s
}

// WithChangeBudget wires the password-change endpoint's
// credential-failure budget, the change twin of WithLoginBudget: the
// surface ChangePassword checks and spends against, keyed by the
// caller. Unwired (a nil surface), the brake is off.
func (s *AuthService) WithChangeBudget(budget *failurebudget.Surface) *AuthService {
	s.changeBudget = budget
	return s
}

// Login verifies the given credentials against the user repository and,
// on success, returns a fresh access token, a new opaque refresh token
// (whose SHA-256 hash is persisted as a session row), and the
// authenticated user.
//
// Failure budget: failed verifications — wrong password and unknown
// username alike — are counted against the username in the request
// body, on the login endpoint's failurebudget.Surface (wired by
// internal/app; the surface owns the namespaced key). Once the budget
// is spent, further fresh logins for that name answer
// ErrRateLimitExceeded (429) before any verification runs, even
// with the correct password; only the window's expiry restores access.
// The budget bars fresh logins, not sessions: Refresh is untouched, so
// an active refresh session keeps working while its username is locked
// out. Successes and storage failures never spend the budget.
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
	if !s.loginBudget.Check(username) {
		return "", "", nil, apperrors.ErrRateLimitExceeded
	}

	user, err = s.userRepo.GetByUsername(username)
	if err != nil {
		return "", "", nil, err
	}
	if user == nil {
		s.loginBudget.Spend(username)
		return "", "", nil, apperrors.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.loginBudget.Spend(username)
		return "", "", nil, apperrors.ErrInvalidCredentials
	}

	accessToken, err = s.GenerateToken(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	if err := s.sessionRepo.DeleteExpiredForUser(user.ID); err != nil {
		return "", "", nil, err
	}

	refreshToken, err = s.mintSession(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, user, nil
}

// Setup is the first-run bootstrap path: on an instance with no users,
// it creates the first user — always an admin, via the shared
// user-creation service's guarded path (FirstUserCreator) — and issues
// the fresh login (access JWT + refresh session) the caller adopts on
// the spot, exactly as at login. Session minting rides the same
// GenerateToken + mintSession primitives ChangePassword uses.
//
// Failure mapping: the creation path's validation failures (missing
// fields, password too short) pass through unchanged, and a losing
// concurrent setup — the winner's transaction committed first — yields
// ErrSetupCompleted (409); in every failure case nothing was written.
// Storage and token-signing failures propagate as-is (the handler maps
// them to a generic 500).
func (s *AuthService) Setup(username, password string) (accessToken, refreshToken string, user *models.User, err error) {
	user, err = s.firstUsers.CreateFirstAdmin(username, password)
	if err != nil {
		return "", "", nil, err
	}

	accessToken, err = s.GenerateToken(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	refreshToken, err = s.mintSession(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, user, nil
}

// Refresh exchanges a presented refresh token for a fresh access JWT, a
// rotated refresh token, and the user the session belongs to. The
// presented token's hash must address a live session row whose user
// still exists; rotation atomically replaces that row with one holding a
// fresh token and a renewed (sliding) expiry, so a token can be redeemed
// exactly once — there is deliberately no non-rotating refresh path.
//
// Failure mapping: an unknown hash — never issued, revoked, already
// rotated (replayed), or belonging to a since-deleted user — yields
// ErrRefreshInvalid (401); a found-but-expired row is deleted on
// encounter as lazy cleanup and yields ErrSessionExpired (401). A
// rotation that loses to a concurrent refresh presenting the same token
// also yields ErrRefreshInvalid: the winner already consumed the row.
// Storage failures propagate unchanged (mapped to 500 by the handler).
func (s *AuthService) Refresh(refreshToken string) (accessToken, newRefreshToken string, user *models.User, err error) {
	tokenHash := HashRefreshToken(refreshToken)

	session, err := s.sessionRepo.GetByTokenHash(tokenHash)
	if err != nil {
		return "", "", nil, err
	}
	if session == nil {
		return "", "", nil, apperrors.ErrRefreshInvalid
	}

	now := time.Now().UTC()
	if !now.Before(session.ExpiresAt) {
		if err := s.sessionRepo.DeleteByTokenHash(tokenHash); err != nil {
			return "", "", nil, err
		}
		return "", "", nil, apperrors.ErrSessionExpired
	}

	user, err = s.userRepo.GetByID(session.UserID)
	if err != nil {
		return "", "", nil, err
	}
	if user == nil {
		return "", "", nil, apperrors.ErrRefreshInvalid
	}

	newRefreshToken, newTokenHash, err := generateRefreshToken()
	if err != nil {
		return "", "", nil, err
	}

	accessToken, err = s.GenerateToken(session.UserID)
	if err != nil {
		return "", "", nil, err
	}

	replacement := s.newSession(session.UserID, newTokenHash)
	rotated, err := s.sessionRepo.Rotate(tokenHash, replacement)
	if err != nil {
		return "", "", nil, err
	}
	if !rotated {
		return "", "", nil, apperrors.ErrRefreshInvalid
	}

	return accessToken, newRefreshToken, user, nil
}

// Logout revokes the refresh session addressed by the presented token:
// the session row is deleted, so that token — and the device holding
// it — can no longer refresh. Revoking an unknown token (never issued,
// already rotated away from, or already revoked) is a no-op rather than
// an error, which keeps logout idempotent; only storage failures
// surface to the caller.
func (s *AuthService) Logout(refreshToken string) error {
	return s.sessionRepo.DeleteByTokenHash(HashRefreshToken(refreshToken))
}

// ChangePassword is the self-service path: the authenticated user
// proves authority with their current password, the new password must
// meet the length policy, and on success the hash is replaced and every
// session of the user — the caller's other devices included — is
// revoked. The returned fresh login (new access JWT + a brand-new
// refresh token whose session is created after the revocation) keeps
// the device making the change signed in; every other device must log
// in again with the new password.
//
// Failure budget: wrong current-password attempts count against the
// caller, on the change-password endpoint's own failurebudget.Surface
// (wired by internal/app; the surface owns the namespaced key, so this
// endpoint's budget is separate from login's). A single wrong attempt
// still answers today's 401; once the budget is spent, further attempts
// answer ErrRateLimitExceeded (429) before any verification runs, until
// the window expires. Only the wrong-password failure spends — an
// unknown user record, a too-short new password, and storage failures
// never do; nor do successes.
//
// Failure mapping: an unknown user record (valid token, deleted user)
// yields ErrUserNotFound (404); a wrong current password yields
// ErrInvalidCredentials (401) with a message naming the current
// password; a new password below models.MinPasswordLength yields
// ErrPasswordTooShort (400) — in all three cases nothing is written.
// Storage failures propagate unchanged (mapped to 500 by the handler).
// There is no transaction across the user and session stores: the write
// order (revoke-all, hash, fresh session) means a failure between steps
// leaves the sessions revoked with the password unchanged — the safe
// side of "signed out but not changed", since a stolen refresh token
// must never outlive the password change; the retry re-runs the
// idempotent revocation before the hash write.
func (s *AuthService) ChangePassword(userID, currentPassword, newPassword string) (accessToken, refreshToken string, user *models.User, err error) {
	if !s.changeBudget.Check(userID) {
		return "", "", nil, apperrors.ErrRateLimitExceeded
	}

	user, err = s.userRepo.GetByID(userID)
	if err != nil {
		return "", "", nil, err
	}
	if user == nil {
		return "", "", nil, apperrors.ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		s.changeBudget.Spend(userID)
		return "", "", nil, apperrors.ErrInvalidCredentials.WithMessage("Current password is incorrect")
	}

	if utf8.RuneCountInString(newPassword) < models.MinPasswordLength {
		return "", "", nil, apperrors.ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", "", nil, err
	}
	user.PasswordHash = string(hash)

	// Revoke before the hash write: a failure here writes nothing, and
	// no failure ordering can leave the new hash live while the
	// (possibly stolen) sessions it was meant to revoke still stand.
	if err := s.sessionRepo.DeleteAllForUser(user.ID); err != nil {
		return "", "", nil, err
	}

	if err := s.userRepo.Update(user); err != nil {
		return "", "", nil, err
	}

	accessToken, err = s.GenerateToken(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	refreshToken, err = s.mintSession(user.ID)
	if err != nil {
		return "", "", nil, err
	}

	return accessToken, refreshToken, user, nil
}

// newSession builds a fresh session row for the user with the session
// TTL slid from now. UTC keeps CreatedAt/ExpiresAt comparable against
// the UTC clock the repository's expiry purge uses, whatever the
// server's zone.
func (s *AuthService) newSession(userID, tokenHash string) *models.Session {
	now := time.Now().UTC()
	return &models.Session{
		ID:        uuid.New().String(),
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: now.Add(s.sessionTTL),
	}
}

// mintSession starts a brand-new session for the user and stores it:
// it mints a fresh opaque refresh token (returned, for the cookie —
// never persisted) and persists its hash as the session row via Create.
// Refresh is the Rotate variant of the same act and builds its
// replacement through newSession instead.
func (s *AuthService) mintSession(userID string) (string, error) {
	refreshToken, tokenHash, err := generateRefreshToken()
	if err != nil {
		return "", err
	}
	if err := s.sessionRepo.Create(s.newSession(userID, tokenHash)); err != nil {
		return "", err
	}
	return refreshToken, nil
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

// ValidateToken validates a JWT token. Its failure vocabulary is
// exactly two AppError sentinels, returned bare: an expired library
// error (typed — errors.Is against jwt.ErrTokenExpired, so a joined
// multi-cause error counts too) yields ErrTokenExpired; every other
// failure — malformed, bad signature, not-yet-valid, unverifiable,
// keyfunc/signing-method error, !token.Valid, claims-cast failure,
// issuer mismatch — fails closed to ErrTokenInvalid. Classification
// happens here, once, by type; callers (the JWT middleware) must never
// re-classify by message string.
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
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.ErrTokenExpired
		}
		return nil, apperrors.ErrTokenInvalid
	}

	// Check if token is valid
	if !token.Valid {
		return nil, apperrors.ErrTokenInvalid
	}

	// Extract claims
	claims, ok := token.Claims.(*TokenClaims)
	if !ok {
		return nil, apperrors.ErrTokenInvalid
	}

	// Verify issuer
	if claims.Issuer != s.issuer {
		return nil, apperrors.ErrTokenInvalid
	}

	return claims, nil
}

// TokenClaims represents JWT claims
type TokenClaims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}
