// Package users owns user management: creating users (hash + exists
// check + insert) behind one service path shared by the admin-only
// POST /api/users endpoint and the integration-test seeding seam,
// creating the first admin (same
// validation and hashing, plus the atomic first-user guard) behind the
// unauthenticated first-run setup endpoint, listing users with a
// role-dependent response shape behind GET /api/users, setting a user's
// role behind the admin-only PUT /api/users/{id}/role (with the
// self-lockout guard), and resetting a user's password behind the
// admin-only PUT /api/users/{id}/password (with the acting-admin
// re-auth, the caller-keyed credential-failure budget, the
// self-lockout guard, and revoke-all-sessions).
package users

import (
	"errors"
	"time"
	"unicode/utf8"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user management operations.
type UserService struct {
	userRepo    models.UserRepositoryInterface
	sessionRepo models.SessionRepositoryInterface
	resetBudget *failurebudget.Surface
}

// NewUserService creates a new UserService. The session repository
// backs the admin password reset's revoke-all-sessions step.
func NewUserService(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface) *UserService {
	return &UserService{userRepo: userRepo, sessionRepo: sessionRepo}
}

// WithFailureBudget wires the reset endpoint's credential-failure
// budget the reset path spends against: the failurebudget.Surface
// (issue 236) whose Check ResetPassword consults before the re-auth
// and whose Spend a failed verification pays — the surface owns the
// endpoint-namespaced key, so this surface's budget is separate from
// the auth surface's. The fluent shape mirrors the auth package's
// budget setters: an optional dependency layered onto the two-argument
// constructor. Unwired, wrong-password attempts are never throttled.
func (s *UserService) WithFailureBudget(budget *failurebudget.Surface) *UserService {
	s.resetBudget = budget
	return s
}

// CreateUser creates a regular user with role "user". See
// CreateUserWithRole for the full contract.
func (s *UserService) CreateUser(username, password string) (*models.User, error) {
	return s.CreateUserWithRole(username, password, models.RoleUser)
}

// requirePresent enforces the two fields every creation path requires:
// username and password must be non-empty. Missing/empty fields are
// ErrMissingRequiredFields, the same code the endpoint validator uses.
func requirePresent(username, password string) error {
	if username == "" || password == "" {
		return apperrors.ErrMissingRequiredFields
	}
	return nil
}

// hashPassword enforces the length policy (ErrPasswordTooShort below
// models.MinPasswordLength) and bcrypt-hashes the password. The
// plaintext never leaves the caller; only the hash is returned.
func hashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < models.MinPasswordLength {
		return "", apperrors.ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CreateUserWithRole creates a user with the given role, bcrypt-hashing
// the password before it is persisted. The plaintext password never
// reaches the repository.
//
// Validation (nothing is written on failure): username and password
// must be non-empty (ErrMissingRequiredFields); the password must meet
// the length policy (ErrPasswordTooShort); the role must be empty —
// meaning the regular "user" default — or one of the known roles
// (ErrInvalidRole). All creation callers (the endpoint, the first-run
// setup endpoint, the integration seeding seam) cross the same path, so
// they cannot drift.
func (s *UserService) CreateUserWithRole(username, password, role string) (*models.User, error) {
	if err := requirePresent(username, password); err != nil {
		return nil, err
	}

	if role == "" {
		role = models.RoleUser
	} else if !models.ValidRole(role) {
		return nil, apperrors.ErrInvalidRole
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	exists, err := s.userRepo.UsernameExists(username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperrors.ErrUsernameTaken
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

// CreateFirstAdmin creates the first user of a fresh instance, always
// with the admin role — the first-run setup path. It rides the same
// validation (requirePresent) and hashing (hashPassword) as
// CreateUserWithRole; only the exists-check differs, replaced by the
// atomic first-user guard: the emptiness check and the insert share one
// write transaction in the repository (models.UserRepository.
// CreateFirstUser), so exactly one of two concurrent setup requests can
// win. The loser learns ErrSetupCompleted (409); a winner is returned
// for the caller to mint a session for. Validation failures reuse the
// creation path's codes — VALIDATION_FAILED for missing fields,
// PASSWORD_TOO_SHORT for a short password — but USERNAME_TAKEN cannot
// fire here, by design: with zero users no username can collide, so the
// first-user guard subsumes the exists-check the creation path runs.
// Nothing is written when validation fails or the guard reports the
// instance already configured.
func (s *UserService) CreateFirstAdmin(username, password string) (*models.User, error) {
	if err := requirePresent(username, password); err != nil {
		return nil, err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: hash,
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}

	created, err := s.userRepo.CreateFirstUser(user)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, apperrors.ErrSetupCompleted
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
// Re-auth (issue 187): the request carries the acting admin's own
// password, verified with the same bcrypt comparison login and
// change password use — against the acting admin (the JWT subject),
// never the target. A miss answers ErrInvalidCredentials (401) with
// the change-password message. The endpoint is otherwise a
// password-guessing oracle for whoever holds an admin token, so failed
// verifications spend the caller-keyed credential-failure budget (the
// reset endpoint's failurebudget.Surface, wired by internal/app): once
// spent, further resets answer ErrRateLimitExceeded (429) before any
// verification runs, even with the correct password, until the window
// expires. Only failures spend — successes and storage failures never
// do.
//
// Self-lockout: an admin targeting themselves is refused with
// ErrSelfPasswordReset (400) before the budget is consulted and before
// any verification runs, so a doomed request burns neither a bcrypt
// compare nor budget — self-service change password is the only self
// path.
//
// Validation (nothing is written before it passes): the target must
// exist (ErrUserNotFound — the lookup runs before the budget check, so
// an unknown target answers 404 unchanged even for a locked-out
// caller; the lookup verifies nothing, so it never spends) and the new
// password must meet the length policy (ErrPasswordTooShort). On
// success the updated user is persisted via read-modify-write, so
// username and role ride through unchanged.
// There is no transaction across the user and session stores: the
// write order (revoke-all, hash) means a failure between steps leaves
// the sessions revoked with the password unchanged — the safe side of
// "signed out but not changed", since a stolen refresh token must
// never outlive the password change; the retry re-runs the idempotent
// revocation before the hash write. Storage failures propagate
// unchanged (mapped to 500 by the handler).
func (s *UserService) ResetPassword(callerID, targetUserID, adminPassword, newPassword string) (*models.User, error) {
	if callerID == targetUserID {
		return nil, apperrors.ErrSelfPasswordReset
	}

	user, err := s.userRepo.GetByID(targetUserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperrors.ErrUserNotFound
	}

	if !s.resetBudget.Check(callerID) {
		return nil, apperrors.ErrRateLimitExceeded
	}

	admin, err := s.userRepo.GetByID(callerID)
	if err != nil {
		return nil, err
	}
	if admin == nil {
		// Unreachable behind RequireAdmin (it resolves the caller's row
		// moments earlier), but a vanished acting admin is the same
		// unknown-caller state the self-service change answers 404 for.
		return nil, apperrors.ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(adminPassword)); err != nil {
		s.resetBudget.Spend(callerID)
		return nil, apperrors.ErrInvalidCredentials.WithMessage("Current password is incorrect")
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = hash

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
