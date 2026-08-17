package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/migrations"
)

// newBootstrapDB opens a temp SQLite database with all migrations
// applied, mirroring what cmd/initdb's main() does before creating the
// bootstrap user.
func newBootstrapDB(t *testing.T) *models.UserRepository {
	t.Helper()

	// migrations.Run reads SQL files from a "migrations/" directory
	// relative to cwd; this package lives at cmd/initdb, so the backend
	// root is two levels up.
	t.Chdir("../..")

	db, err := models.NewDatabase(filepath.Join(t.TempDir(), "bootstrap.db"))
	require.NoError(t, err, "Should open temp database")
	require.NoError(t, migrations.Run(db), "Should run migrations")
	t.Cleanup(func() { _ = db.Close() })

	return models.NewUserRepository(db)
}

// TestCreateAdminUser_GrantsAdminRole asserts the bootstrap tool creates
// its user with the admin role — this is what lets the bootstrap user
// later exercise admin-only endpoints.
func TestCreateAdminUser_GrantsAdminRole(t *testing.T) {
	repo := newBootstrapDB(t)

	err := createAdminUser(repo, "bootstrap-admin", "bootstrap-password")
	require.NoError(t, err, "Should create bootstrap admin without error")

	user, err := repo.GetByUsername("bootstrap-admin")
	require.NoError(t, err, "Should look up bootstrap user")
	require.NotNil(t, user, "Bootstrap user should exist after create")
	assert.Equal(t, models.RoleAdmin, user.Role, "Bootstrap user must have the admin role")
	assert.NotEqual(t, "bootstrap-password", user.PasswordHash, "Password must be stored hashed")
}

// TestCreateAdminUser_IdempotentRerun asserts re-running the bootstrap
// tool against an already-created admin leaves the user untouched: no
// error, no duplicate row, role and password hash unchanged.
func TestCreateAdminUser_IdempotentRerun(t *testing.T) {
	repo := newBootstrapDB(t)

	require.NoError(t, createAdminUser(repo, "bootstrap-admin", "first-password"), "First run should succeed")

	before, err := repo.GetByUsername("bootstrap-admin")
	require.NoError(t, err, "Should look up bootstrap user after first run")
	require.NotNil(t, before, "Bootstrap user should exist after first run")

	require.NoError(t, createAdminUser(repo, "bootstrap-admin", "second-password"), "Re-run against existing admin should be a no-op success")

	after, err := repo.GetByUsername("bootstrap-admin")
	require.NoError(t, err, "Should look up bootstrap user after re-run")
	require.NotNil(t, after, "Bootstrap user must still exist after re-run")

	assert.Equal(t, before.ID, after.ID, "Re-run must not recreate the user")
	assert.Equal(t, before.PasswordHash, after.PasswordHash, "Re-run must not change the stored password")
	assert.Equal(t, models.RoleAdmin, after.Role, "Re-run must not change the admin role")

	exists, err := repo.UsernameExists("bootstrap-admin")
	require.NoError(t, err, "Should check username existence")
	require.True(t, exists, "Bootstrap user must exist")
}
