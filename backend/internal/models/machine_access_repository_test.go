package models

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAccessDB is newTestDB (in user_repository_test.go) with this
// suite's filename, so access repository tests exercise the real schema
// (including the permission CHECK constraint).
func newTestAccessDB(t *testing.T) *sql.DB {
	t.Helper()
	return newTestDB(t, "access.db")
}

// seedAccessFixture inserts one owner, one machine, and one access row
// with the given permission, returning the access repository against
// the same DB.
func seedAccessFixture(t *testing.T, db *sql.DB, permission string) *MachineAccessRepository {
	t.Helper()

	userRepo := NewUserRepository(db)
	owner := &User{
		ID:           "access-owner-1",
		Username:     "accessowner",
		PasswordHash: "hash",
		Role:         RoleUser,
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, userRepo.Create(owner), "Should create fixture user")

	machineRepo := NewMachineRepository(db)
	machine := &Machine{
		ID:              "access-machine-1",
		Name:            "access-machine",
		OwnerID:         owner.ID,
		Hostname:        "example.test",
		Port:            22,
		SSHUsername:     "core",
		SSHKeyEncrypted: []byte("ciphertext"),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	require.NoError(t, machineRepo.Create(machine), "Should create fixture machine")

	accessRepo := NewMachineAccessRepository(db)
	if permission != "" {
		require.NoError(t, accessRepo.Create(&MachineAccess{
			MachineID:  machine.ID,
			UserID:     owner.ID,
			Permission: permission,
			GrantedAt:  time.Now().UTC(),
		}), "Should create fixture access row")
	}
	return accessRepo
}

// TestMachineAccessRepository_GetPermission_ReturnsStoredLevel asserts
// the stored permission level round-trips for every level the schema's
// CHECK constraint allows. Level-aware gates (RequireMachinePermission)
// base their allow/deny on exactly this value.
func TestMachineAccessRepository_GetPermission_ReturnsStoredLevel(t *testing.T) {
	for _, level := range []string{PermissionRead, PermissionWrite, PermissionAdmin} {
		t.Run(level, func(t *testing.T) {
			db := newTestAccessDB(t)
			accessRepo := seedAccessFixture(t, db, level)

			got, err := accessRepo.GetPermission("access-machine-1", "access-owner-1")
			require.NoError(t, err, "GetPermission must not fail for a stored row")
			assert.Equal(t, level, got, "GetPermission must return the stored level")
		})
	}
}

// TestMachineAccessRepository_GetPermission_EmptyWhenNoRow asserts the
// no-row case surfaces as the empty string (not an error), which is the
// contract HasAtLeast consumes: missing access is just the bottom of
// the ladder.
func TestMachineAccessRepository_GetPermission_EmptyWhenNoRow(t *testing.T) {
	db := newTestAccessDB(t)
	accessRepo := seedAccessFixture(t, db, "")

	got, err := accessRepo.GetPermission("access-machine-1", "access-owner-1")
	require.NoError(t, err, "GetPermission must treat a missing row as no error")
	assert.Empty(t, got, "GetPermission must return the empty string when no row exists")
}
