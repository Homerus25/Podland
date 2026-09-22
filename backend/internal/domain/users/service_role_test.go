package users

import (
	"fmt"
	"testing"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleTestUsers seeds one admin and two regular users with fixed IDs
// into a fresh mock repository wired to a UserService.
func roleTestUsers(t *testing.T) (*UserService, *testutils.MockUserRepository) {
	t.Helper()

	repo := testutils.NewMockUserRepository()
	created := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	repo.AddUser(&models.User{ID: "admin-1", Username: "root", Role: models.RoleAdmin, CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-1", Username: "alice", Role: models.RoleUser, CreatedAt: created})
	repo.AddUser(&models.User{ID: "user-2", Username: "bob", Role: models.RoleUser, CreatedAt: created})

	return NewUserService(repo, testutils.NewMockSessionRepository()), repo
}

// storedRole returns the persisted role of the user with the given ID.
func storedRole(t *testing.T, repo *testutils.MockUserRepository, userID string) string {
	t.Helper()

	user, err := repo.GetByID(userID)
	require.NoError(t, err, "Should read the user back from the repository")
	require.NotNil(t, user, "User %q should exist in the repository", userID)
	return user.Role
}

// TestUserService_SetRole_PromotesAndDemotes asserts the happy path in
// both directions: an admin promotes a regular user to admin and demotes
// them back, and the change is persisted.
func TestUserService_SetRole_PromotesAndDemotes(t *testing.T) {
	service, repo := roleTestUsers(t)

	promoted, err := service.SetRole("admin-1", "user-1", models.RoleAdmin)
	require.NoError(t, err, "Promotion should succeed")
	require.NotNil(t, promoted, "The updated user should be returned")
	assert.Equal(t, models.RoleAdmin, promoted.Role, "The returned user should carry the new role")
	assert.Equal(t, models.RoleAdmin, storedRole(t, repo, "user-1"), "The promotion should be persisted")

	demoted, err := service.SetRole("admin-1", "user-1", models.RoleUser)
	require.NoError(t, err, "Demotion should succeed")
	assert.Equal(t, models.RoleUser, demoted.Role, "The returned user should carry the demoted role")
	assert.Equal(t, models.RoleUser, storedRole(t, repo, "user-1"), "The demotion should be persisted")
}

// TestUserService_SetRole_SelfLockoutGuard pins the guard: an admin
// changing their own role is rejected with SELF_ROLE_CHANGE and nothing
// is written — the instance can never lose its administration to a
// misclick (the bootstrap CLI remains the escape hatch).
func TestUserService_SetRole_SelfLockoutGuard(t *testing.T) {
	service, repo := roleTestUsers(t)

	updated, err := service.SetRole("admin-1", "admin-1", models.RoleUser)

	require.Error(t, err, "An admin changing their own role must be rejected")
	assert.Nil(t, updated, "No user is returned for a rejected self-change")
	assert.Equal(t, apperrors.ErrSelfRoleChange, err, "The self-change should surface SELF_ROLE_CHANGE")
	assert.Equal(t, models.RoleAdmin, storedRole(t, repo, "admin-1"), "The caller's role must be untouched")
}

// TestUserService_SetRole_InvalidRole asserts the role is validated to
// {user, admin} and nothing is written on rejection.
func TestUserService_SetRole_InvalidRole(t *testing.T) {
	service, repo := roleTestUsers(t)

	for _, role := range []string{"superuser", "ADMIN", "user ", ""} {
		updated, err := service.SetRole("admin-1", "user-1", role)
		require.Error(t, err, "Role %q must be rejected", role)
		assert.Nil(t, updated, "No user is returned for an invalid role")
		assert.Equal(t, apperrors.ErrInvalidRole, err, "Role %q should surface VALIDATION_FAILED", role)
	}

	assert.Equal(t, models.RoleUser, storedRole(t, repo, "user-1"), "The target's role must be untouched")
}

// TestUserService_SetRole_TargetNotFound asserts changing the role of a
// user that does not exist is answered with USER_NOT_FOUND (404).
func TestUserService_SetRole_TargetNotFound(t *testing.T) {
	service, _ := roleTestUsers(t)

	updated, err := service.SetRole("admin-1", "ghost-id", models.RoleAdmin)

	require.Error(t, err, "A missing target user must be rejected")
	assert.Nil(t, updated, "No user is returned for a missing target")
	assert.Equal(t, apperrors.ErrUserNotFound, err, "A missing target should surface USER_NOT_FOUND")
}

// TestUserService_SetRole_RepoError asserts a failing update propagates
// (the envelope answers it with the 500 fallback).
func TestUserService_SetRole_RepoError(t *testing.T) {
	service, repo := roleTestUsers(t)
	repo.WithUpdateError(fmt.Errorf("disk on fire"))

	_, err := service.SetRole("admin-1", "user-1", models.RoleAdmin)

	require.Error(t, err, "A failing update must fail the call")
	assert.Contains(t, err.Error(), "disk on fire", "The repository cause should propagate")
	assert.Equal(t, models.RoleUser, storedRole(t, repo, "user-1"), "A failed update must leave the role untouched")
}
