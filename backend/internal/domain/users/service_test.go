package users

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/Homerus25/podland/backend/migrations"
	"golang.org/x/crypto/bcrypt"
)

// TestUserService_CreateUser_HashesPassword asserts the tracer bullet:
// CreateUser persists a user whose password is stored bcrypt-hashed (the
// plaintext never reaches the repository) and whose role is the regular
// "user" role — the endpoint never grants anything else.
func TestUserService_CreateUser_HashesPassword(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo)

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
	service := NewUserService(repo)

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
	service := NewUserService(repo)

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

	service := NewUserService(repo)
	_, err = service.CreateUser("raced-bob", "some-password")

	require.Error(t, err, "Creation colliding with an existing row must fail")
	assert.Equal(t, apperrors.ErrUsernameTaken, err, "Real UNIQUE violation should surface as USERNAME_TAKEN (409)")
}

// TestUserService_CreateUser_MissingFields asserts empty username or
// password is rejected with VALIDATION_FAILED before any hashing or
// persistence happens.
func TestUserService_CreateUser_MissingFields(t *testing.T) {
	repo := testutils.NewMockUserRepository()
	service := NewUserService(repo)

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
