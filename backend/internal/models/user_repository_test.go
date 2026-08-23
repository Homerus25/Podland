package models

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/migrations"
)

// newTestUserDB opens a temp SQLite database and runs the real
// migrations, so user repository tests exercise the actual schema
// (including column defaults) rather than hand-rolled CREATE TABLEs.
func newTestUserDB(t *testing.T) *sql.DB {
	t.Helper()

	// migrations.Run reads SQL files from a "migrations/" directory
	// relative to cwd; this package lives at internal/models, so the
	// backend root is two levels up (same approach as app_test.go).
	t.Chdir("../..")

	db, err := NewDatabase(filepath.Join(t.TempDir(), "users.db"))
	require.NoError(t, err, "Should open temp database")
	require.NoError(t, migrations.Run(db), "Should run migrations")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestUserRepository_RoleRoundTrip asserts that a user created with the
// admin role is read back with that role, by both lookup paths (ID and
// username). Authorization decisions downstream depend on the role
// surviving a write→read cycle unchanged.
func TestUserRepository_RoleRoundTrip(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewUserRepository(db)

	user := &User{
		ID:           "user-role-1",
		Username:     "roleadmin",
		PasswordHash: "hash",
		Role:         RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.Create(user), "Should create admin-role user")

	byUsername, err := repo.GetByUsername("roleadmin")
	require.NoError(t, err, "Should get user by username")
	require.NotNil(t, byUsername, "Created user should be findable by username")
	assert.Equal(t, RoleAdmin, byUsername.Role, "Role should round-trip via GetByUsername")

	byID, err := repo.GetByID("user-role-1")
	require.NoError(t, err, "Should get user by ID")
	require.NotNil(t, byID, "Created user should be findable by ID")
	assert.Equal(t, RoleAdmin, byID.Role, "Role should round-trip via GetByID")
}

// TestUserRepository_Create_RejectsUnknownRole asserts the
// application-level role constraint: values outside {user, admin} are
// rejected at the repository boundary instead of being persisted.
func TestUserRepository_Create_RejectsUnknownRole(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewUserRepository(db)

	user := &User{
		ID:           "user-role-2",
		Username:     "malformed",
		PasswordHash: "hash",
		Role:         "superuser",
		CreatedAt:    time.Now().UTC(),
	}

	err := repo.Create(user)
	require.Error(t, err, "Create should reject a role outside {user, admin}")
	assert.Contains(t, err.Error(), "role", "Error should mention the role")

	stored, err := repo.GetByUsername("malformed")
	require.NoError(t, err, "Should get user by username")
	assert.Nil(t, stored, "User with invalid role must not be persisted")
}

// TestUserRepository_Update_RejectsEmptyRole asserts Update demands an
// explicit role: a hand-built User with Role unset must fail rather
// than silently demote an admin back to "user".
func TestUserRepository_Update_RejectsEmptyRole(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewUserRepository(db)

	created := &User{
		ID:           "user-role-5",
		Username:     "demoteguards",
		PasswordHash: "hash",
		Role:         RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.Create(created), "Should create admin user")

	handBuilt := &User{
		ID:           created.ID,
		Username:     "demoteguards",
		PasswordHash: "new-hash",
		CreatedAt:    created.CreatedAt,
	}
	err := repo.Update(handBuilt)
	require.Error(t, err, "Update should reject an unset role instead of defaulting it")

	stored, err := repo.GetByID(created.ID)
	require.NoError(t, err, "Should get user by ID")
	require.NotNil(t, stored, "User should still exist")
	assert.Equal(t, RoleAdmin, stored.Role, "Failed update must leave the admin role untouched")
}

// TestUserRepository_Update_PreservesRoleThroughRoundTrip asserts the
// read-modify-write flow keeps the role: fetch, change the password,
// save, reload — the role must survive.
func TestUserRepository_Update_PreservesRoleThroughRoundTrip(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewUserRepository(db)

	created := &User{
		ID:           "user-role-6",
		Username:     "keepmyrole",
		PasswordHash: "old-hash",
		Role:         RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.Create(created), "Should create admin user")

	fetched, err := repo.GetByUsername("keepmyrole")
	require.NoError(t, err, "Should get user by username")
	fetched.PasswordHash = "new-hash"
	require.NoError(t, repo.Update(fetched), "Should update fetched user")

	reloaded, err := repo.GetByID(created.ID)
	require.NoError(t, err, "Should get user by ID")
	require.NotNil(t, reloaded, "User should still exist")
	assert.Equal(t, RoleAdmin, reloaded.Role, "Role must survive a read-modify-write cycle")
	assert.Equal(t, "new-hash", reloaded.PasswordHash, "Password hash should be updated")
}

// TestUserRepository_MigrationDefaultsRoleToUser pins the migration
// default: a row inserted without a role (the shape every pre-migration
// insert had, and what raw SQL writes look like) must read back as
// 'user'. This exercises the SQL DEFAULT, not the repository's
// normalization, by bypassing repo.Create entirely.
func TestUserRepository_MigrationDefaultsRoleToUser(t *testing.T) {
	db := newTestUserDB(t)

	_, err := db.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		"user-role-4", "preexisting", "hash", time.Now().UTC(),
	)
	require.NoError(t, err, "Should insert a role-less row like pre-migration code did")

	repo := NewUserRepository(db)
	stored, err := repo.GetByUsername("preexisting")
	require.NoError(t, err, "Should get user by username")
	require.NotNil(t, stored, "Inserted user should be findable")
	assert.Equal(t, RoleUser, stored.Role, "Rows without an explicit role must default to user")
}

// TestUserRepository_Create_DefaultsEmptyRoleToUser asserts that callers
// which leave Role unset (the pre-role code shape) get the same default
// the database schema applies, rather than an empty string.
func TestUserRepository_Create_DefaultsEmptyRoleToUser(t *testing.T) {
	db := newTestUserDB(t)
	repo := NewUserRepository(db)

	user := &User{
		ID:           "user-role-3",
		Username:     "legacyshape",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.Create(user), "Should create user with unset role")

	stored, err := repo.GetByUsername("legacyshape")
	require.NoError(t, err, "Should get user by username")
	require.NotNil(t, stored, "Created user should be findable")
	assert.Equal(t, RoleUser, stored.Role, "Unset role should default to user")
}
