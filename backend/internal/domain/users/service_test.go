package users

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/Homerus25/podland/backend/migrations"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestUserService_CreateUser_HashesPassword asserts the tracer bullet:
// CreateUser persists a user whose password is stored bcrypt-hashed (the
// plaintext never reaches the repository) and whose role is the regular
// "user" role — the endpoint never grants anything else.
func TestUserService_CreateUser_HashesPassword(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo, testutils.NewMockSessionRepository())

	user, err := service.CreateUser("alice", "correct-horse-battery")

	require.NoError(t, err, "Should create user without error")
	require.NotNil(t, user, "Created user should be returned")

	assert.NotEmpty(t, user.ID, "Created user should have an ID")
	assert.Equal(t, "alice", user.Username, "Created user should carry the requested username")
	assert.Equal(t, models.RoleUser, user.Role, "CreateUser must grant the regular user role")

	require.NotEmpty(t, user.PasswordHash, "Created user should have a password hash")
	assert.NotEqual(t, "correct-horse-battery", user.PasswordHash, "Password must never be stored in plaintext")
	assert.NoError(t,
		bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("correct-horse-battery")),
		"Stored hash must verify against the given password")
}

// TestUserService_CreateUser_DuplicateUsername asserts creating a user
// whose username already exists fails with the USERNAME_TAKEN error (the
// HTTP layer surfaces it as 409).
func TestUserService_CreateUser_DuplicateUsername(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo, testutils.NewMockSessionRepository())

	_, err := service.CreateUser("alice", "first-password")
	require.NoError(t, err, "Should create the first user")

	_, err = service.CreateUser("alice", "second-password")
	require.Error(t, err, "Duplicate username must be rejected")
	assert.Equal(t, apperrors.ErrUsernameTaken, err, "Duplicate username should surface USERNAME_TAKEN")
}

// TestUserService_CreateUser_UniqueConstraintBackstop asserts that a
// UNIQUE-violation from the INSERT — what happens when two concurrent
// creations race past the exists-check — surfaces as USERNAME_TAKEN
// rather than an opaque 500.
func TestUserService_CreateUser_UniqueConstraintBackstop(t *testing.T) {
	// The driver surfaces constraint failures as sqlite3.Error values
	// (value receiver), and the repository wraps them with %w.
	constraintErr := fmt.Errorf("failed to create user: %w", sqlite3.Error{
		Code:         sqlite3.ErrConstraint,
		ExtendedCode: sqlite3.ErrConstraintUnique,
	})
	repo := testutils.NewMockUserRepositoryWithErrors(constraintErr, nil, nil, nil, nil, nil)
	service := NewUserService(repo, testutils.NewMockSessionRepository())

	_, err := service.CreateUser("raced-alice", "some-password")

	require.Error(t, err, "Constraint violation must fail the creation")
	assert.Equal(t, apperrors.ErrUsernameTaken, err, "UNIQUE violation should surface as USERNAME_TAKEN (409)")
}

// TestUserService_CreateUser_UniqueConstraintBackstop_RealDB proves the
// backstop against the real SQLite constraint: a row inserted directly
// through the repository (bypassing the service's exists-check, like a
// racing concurrent creation would) makes the service's INSERT trip the
// users.username UNIQUE constraint, which must surface as
// USERNAME_TAKEN rather than an opaque 500.
func TestUserService_CreateUser_UniqueConstraintBackstop_RealDB(t *testing.T) {
	// migrations.Run reads SQL files from a "migrations/" directory
	// relative to cwd; this package lives at internal/domain/users, so
	// the backend root is three levels up.
	t.Chdir("../../..")

	db, err := models.NewDatabase(filepath.Join(t.TempDir(), "users-service.db"))
	require.NoError(t, err, "Should open temp database")
	require.NoError(t, migrations.Run(db), "Should run migrations")
	t.Cleanup(func() { _ = db.Close() })

	repo := models.NewUserRepository(db)
	require.NoError(t, repo.Create(&models.User{
		ID:           "racer-1",
		Username:     "raced-bob",
		PasswordHash: "some-hash",
		Role:         models.RoleUser,
		CreatedAt:    time.Now().UTC(),
	}), "Should insert the racing winner directly via the repository")

	service := NewUserService(repo, testutils.NewMockSessionRepository())
	_, err = service.CreateUser("raced-bob", "some-password")

	require.Error(t, err, "Creation colliding with an existing row must fail")
	assert.Equal(t, apperrors.ErrUsernameTaken, err, "Real UNIQUE violation should surface as USERNAME_TAKEN (409)")
}

// TestUserService_CreateUser_MissingFields asserts empty username or
// password is rejected with VALIDATION_FAILED before any hashing or
// persistence happens.
func TestUserService_CreateUser_MissingFields(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo, testutils.NewMockSessionRepository())

	_, err := service.CreateUser("", "some-password")
	assert.Equal(t, apperrors.ErrMissingRequiredFields, err, "Empty username should be rejected")

	_, err = service.CreateUser("bob", "")
	assert.Equal(t, apperrors.ErrMissingRequiredFields, err, "Empty password should be rejected")

	for _, username := range []string{"", "bob"} {
		exists, existsErr := repo.UsernameExists(username)
		require.NoError(t, existsErr, "Should check the mock repository")
		assert.False(t, exists, "No user may be persisted for invalid input (username %q)", username)
	}
}

// TestUserService_CreateUserWithRole_RolesAndPolicy pins the shared
// creation path's validation: an empty role defaults to the regular
// "user" role, "admin" is accepted, an unknown role is rejected with
// VALIDATION_FAILED, and the password policy holds on every path.
func TestUserService_CreateUserWithRole_RolesAndPolicy(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo, testutils.NewMockSessionRepository())

	defaulted, err := service.CreateUserWithRole("defaults", "some-password", "")
	require.NoError(t, err, "An empty role should default")
	assert.Equal(t, models.RoleUser, defaulted.Role, "An omitted role means the regular user role")

	admin, err := service.CreateUserWithRole("boss", "some-password", models.RoleAdmin)
	require.NoError(t, err, "The admin role should be accepted")
	assert.Equal(t, models.RoleAdmin, admin.Role, "The requested role should be persisted")

	for _, role := range []string{"superuser", "ADMIN", "user "} {
		_, err := service.CreateUserWithRole("doomed-"+role, "some-password", role)
		require.Error(t, err, "Unknown role %q must be rejected", role)
		assert.Equal(t, apperrors.ErrInvalidRole, err, "Unknown role %q should surface VALIDATION_FAILED", role)
	}

	exists, err := repo.UsernameExists("doomed-superuser")
	require.NoError(t, err, "Should check the mock repository")
	assert.False(t, exists, "No user may be persisted for an invalid role")

	tooShort, err := service.CreateUserWithRole("shorty", "short", models.RoleUser)
	require.Error(t, err, "A password below the policy must be rejected")
	assert.Nil(t, tooShort, "No user is returned for a rejected password")
	assert.Equal(t, apperrors.ErrPasswordTooShort, err, "A short password should surface PASSWORD_TOO_SHORT")

	_, err = service.CreateUserWithRole("seven", "1234567", models.RoleUser)
	require.Error(t, err, "A password one rune below the policy must also be rejected")
	assert.Equal(t, apperrors.ErrPasswordTooShort, err, "7 characters should surface PASSWORD_TOO_SHORT (the policy floor is 8)")

	exactly, err := service.CreateUserWithRole("boundary", "12345678", models.RoleUser)
	require.NoError(t, err, "A password of exactly the minimum length should be accepted")
	assert.Equal(t, "boundary", exactly.Username, "The boundary-length user is created")
}

// listTestUsers seeds one admin and two regular users with fixed IDs
// into a fresh mock repository wired to a UserService.
func listTestUsers(t *testing.T) (*UserService, *testutils.MockUserRepository, []string) {
	t.Helper()

	repo := testutils.NewMockUserRepository()
	created := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	repo.AddUser(&models.User{ID: "admin-1", Username: "root", Role: models.RoleAdmin, CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-1", Username: "alice", Role: models.RoleUser, CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-2", Username: "bob", Role: models.RoleUser, CreatedAt: created})

	return NewUserService(repo, testutils.NewMockSessionRepository()), repo, []string{"admin-1", "user-1", "user-2"}
}

// TestUserService_List_AdminCaller asserts the service contract: every
// stored user comes back as a domain row — the hash-carrying one
// included, listed like any other (that the wire never exposes the hash
// is the handler's DTO mapping, pinned at the HTTP surface in
// handler_test.go) — and the admin flag is true for an admin caller.
func TestUserService_List_AdminCaller(t *testing.T) {
	service, repo, _ := listTestUsers(t)
	repo.AddUser(&models.User{ID: "hashy", Username: "hash-carrier", Role: models.RoleUser,
		PasswordHash: "super-secret-bcrypt-hash", CreatedAt: time.Now()})

	users, callerIsAdmin, err := service.List("admin-1")

	require.NoError(t, err, "Admin list should succeed")
	assert.True(t, callerIsAdmin, "An admin caller should be flagged as admin")
	require.Len(t, users, 4, "Every user should be listed")

	byID := make(map[string]*models.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}

	assert.Equal(t, "root", byID["admin-1"].Username, "Admin entry should carry the username")
	assert.Equal(t, models.RoleAdmin, byID["admin-1"].Role, "Admin entries should carry the role")
	assert.Equal(t, "alice", byID["user-1"].Username, "Regular user entries should carry the username")
	assert.Equal(t, "hash-carrier", byID["hashy"].Username, "The hash-carrying row is listed like any other")
	assert.Equal(t, models.RoleUser, byID["hashy"].Role, "The hash-carrying row keeps its role")
}

// TestUserService_List_UserCallerNotAdmin asserts a regular-user caller
// is flagged non-admin but still receives every user — the admin
// included, so the machine-sharing picker can discover them. The
// minimal wire shape for this caller is the handler's DTO mapping.
func TestUserService_List_UserCallerNotAdmin(t *testing.T) {
	service, _, ids := listTestUsers(t)

	users, callerIsAdmin, err := service.List("user-1")

	require.NoError(t, err, "Regular-user list should succeed")
	assert.False(t, callerIsAdmin, "A regular caller should not be flagged as admin")
	require.Len(t, users, len(ids), "Every user should be listed")

	byID := make(map[string]string, len(users))
	for _, user := range users {
		byID[user.ID] = user.Username
	}
	assert.Equal(t, "root", byID["admin-1"], "The admin should be discoverable by id")
	assert.Equal(t, "bob", byID["user-2"], "Fellow users should be discoverable by id")
	assert.Equal(t, "alice", byID["user-1"], "The caller is listed like everyone else")
}

// TestUserService_List_MissingCaller asserts a caller whose user record
// no longer exists is answered with ErrAdminRequired (403), matching
// RequireAdmin's treatment of the same state.
func TestUserService_List_MissingCaller(t *testing.T) {
	service, _, _ := listTestUsers(t)

	_, _, err := service.List("ghost-id")

	require.Error(t, err, "A caller without a user record must be rejected")
	assert.Equal(t, apperrors.ErrAdminRequired, err, "Missing caller should surface ADMIN_REQUIRED (403)")
}

// TestUserService_List_RepoError asserts a failing list query propagates
// (the envelope answers it with the 500 fallback).
func TestUserService_List_RepoError(t *testing.T) {
	service, repo, _ := listTestUsers(t)
	repo.WithListError(fmt.Errorf("disk on fire"))

	_, _, err := service.List("admin-1")

	require.Error(t, err, "A failing list query must fail the call")
	assert.Contains(t, err.Error(), "disk on fire", "The repository cause should propagate")
}
