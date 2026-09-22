// Package users owns user management: creating users (hash + exists
// check + insert) behind one service path shared by the admin-only
// POST /api/users endpoint, the cmd/initdb bootstrap tool, and the
// integration-test seeding seam, listing users with a role-dependent
// response shape behind GET /api/users, setting a user's role behind
// the admin-only PUT /api/users/{id}/role (with the self-lockout
// guard), and resetting a user's password behind the admin-only
// PUT /api/users/{id}/password (with revoke-all-sessions).
package users

import (
	"errors"
	"time"
	"unicode/utf8"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user management operations.
type UserService struct {
	userRepo    models.UserRepositoryInterface
	sessionRepo models.SessionRepositoryInterface
}

// NewUserService creates a new UserService. The session repository
// backs the admin password reset's revoke-all-sessions step.
func NewUserService(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface) *UserService {
	return &UserService{userRepo: userRepo, sessionRepo: sessionRepo}
}

// CreateUser creates a regular user with role "user". See
// CreateUserWithRole for the full contract.
func (s *UserService) CreateUser(username, password string) (*models.User, error) {
	return s.CreateUserWithRole(username, password, models.RoleUser)
}

// CreateUserWithRole creates a user with the given role, bcrypt-hashing
// the password before it is persisted. The plaintext password never
// reaches the repository.
//
// Validation (nothing is written on failure): username and password
// must be non-empty (ErrMissingRequiredFields); the password must meet
// the length policy (ErrPasswordTooShort); the role must be empty —
// meaning the regular "user" default — or one of the known roles
// (ErrInvalidRole). Bootstrap callers (cmd/initdb, the integration
// seeding seam) cross the same path, so endpoint, tool, and harness
// cannot drift.
func (s *UserService) CreateUserWithRole(username, password, role string) (*models.User, error) {
	if username == "" || password == "" {
		return nil, apperrors.ErrMissingRequiredFields
	}

	if role == "" {
		role = models.RoleUser
	} else if !models.ValidRole(role) {
		return nil, apperrors.ErrInvalidRole
	}

	if utf8.RuneCountInString(password) < models.MinPasswordLength {
		return nil, apperrors.ErrPasswordTooShort
	}

	exists, err := s.userRepo.UsernameExists(username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.userRepo.Create(user); err != nil {
		// Backstop for the concurrent-duplicate race: two creations can
		// pass the exists-check before either INSERT lands, and the
		// loser then trips the users.username UNIQUE constraint.
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) &&
			sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return nil, apperrors.ErrUsernameTaken
		}
		return nil, err
	}

	return user, nil
}

// SetRole sets the target user's role to one of the known roles
// ("user" or "admin" — anything else is ErrInvalidRole). The
// self-lockout guard rejects a caller changing their own role
// (ErrSelfRoleChange): one misclick must never strip the instance of
// its administration; the bootstrap CLI remains the escape hatch. A
// target without a user record is ErrUserNotFound. On success the
// updated user is returned and persisted via read-modify-write, so
// username and password hash ride through unchanged.
func (s *UserService) SetRole(callerID, targetUserID, role string) (*models.User, error) {
	if !models.ValidRole(role) {
		return nil, apperrors.ErrInvalidRole
	}

	if callerID == targetUserID {
		return nil, apperrors.ErrSelfRoleChange
	}

	user, err := s.userRepo.GetByID(targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperrors.ErrUserNotFound
	}

	user.Role = role
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

// ResetPassword is the admin path: an admin replaces the target user's
// password hash and revokes every session of that user, so stolen
// refresh tokens die with the old password and take effect
// immediately. Unlike the self-service change (auth
// ChangePassword), no fresh login is issued — the caller is the
// admin, not the target, and the target must log in with the new
// password. Reusing the revoke-all-sessions primitive keeps both paths
// on one revocation implementation.
//
// Validation (nothing is written before it passes): the target must
// exist (ErrUserNotFound) and the new password must meet the length
// policy (ErrPasswordTooShort). On success the updated user is
// persisted via read-modify-write, so username and role ride through
// unchanged. There is no transaction across the user and session
// stores: the write order (revoke-all, hash) means a failure between
// steps leaves the sessions revoked with the password unchanged — the
// safe side of "signed out but not changed", since a stolen refresh
// token must never outlive the password change; the retry re-runs the
// idempotent revocation before the hash write. Storage failures
// propagate unchanged (mapped to 500 by the handler).
func (s *UserService) ResetPassword(targetUserID, newPassword string) (*models.User, error) {
	user, err := s.userRepo.GetByID(targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperrors.ErrUserNotFound
	}

	if utf8.RuneCountInString(newPassword) < models.MinPasswordLength {
		return nil, apperrors.ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = string(hash)

	// Revoke before the hash write: a failure here writes nothing, and
	// no failure ordering can leave the new hash live while the
	// (possibly stolen) sessions it was meant to revoke still stand.
	if err := s.sessionRepo.DeleteAllForUser(user.ID); err != nil {
		return nil, err
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

// List returns the Podland users plus whether the caller is an admin —
// the handler uses the flag to pick the wire shape (the full
// UserResponse for admins, the minimal UserSummaryResponse for every
// other authenticated caller). The caller's role is resolved from the
// database on every request, mirroring middleware.RequireAdmin's
// per-request ruling, so a role change takes effect immediately rather
// than at token expiry; a caller whose user record no longer exists is
// answered with ErrAdminRequired, matching that middleware's treatment
// of the same state. The returned rows are domain records — the wire
// shaping is the handler's, like on every sibling endpoint.
func (s *UserService) List(callerID string) (users []*models.User, callerIsAdmin bool, err error) {
	caller, err := s.userRepo.GetByID(callerID)
	if err != nil {
		return nil, false, err
	}
	if caller == nil {
		return nil, false, apperrors.ErrAdminRequired
	}

	users, err = s.userRepo.List()
	if err != nil {
		return nil, false, err
	}
	return users, caller.Role == models.RoleAdmin, nil
}
