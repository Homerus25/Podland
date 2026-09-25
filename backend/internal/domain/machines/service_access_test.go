package machines

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAccessTestService builds a machine service over mock repos with one
// machine (id "machine-1", owner "owner-1") and the owner's admin access
// row, mirroring production registration.
func newAccessTestService() (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineService(machineRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	machineRepo.AddMachine(&models.Machine{
		ID:                "machine-1",
		Name:              "shared-machine",
		Hostname:          "machine.example.com",
		Port:              22,
		SSHUsername:       "user",
		SSHKeyEncrypted:   []byte("encrypted"),
		SSHKeyFingerprint: "fingerprint",
		PodmanSocketPath:  "/run/podman/podman.sock",
		OwnerID:           "owner-1",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})
	accessRepo.AddAccess("machine-1", "owner-1", models.PermissionAdmin, time.Now().UTC())
	return service, machineRepo, accessRepo
}

// TestMachineService_GrantAccess covers the POST /api/machines/{id}/access
// rules at the service seam: owner-only grant over mock repositories.
func TestMachineService_GrantAccess(t *testing.T) {
	t.Run("owner grants a new user and the row persists", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "machine-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionRead,
		})

		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Equal(t, "machine-1", access.MachineID)
		assert.Equal(t, "reader-1", access.UserID)
		assert.Equal(t, models.PermissionRead, access.Permission)
		assert.False(t, access.GrantedAt.IsZero())

		stored, err := accessRepo.GetByMachineAndUser("machine-1", "reader-1")
		require.NoError(t, err)
		require.NotNil(t, stored, "granted access row must persist")
		assert.Equal(t, models.PermissionRead, stored.Permission)
	})

	t.Run("invalid permission is rejected", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "machine-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: "superuser",
		})

		var appErr *errors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "INVALID_PERMISSION", appErr.Code)
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
		assert.Nil(t, access)
		stored, err := accessRepo.GetByMachineAndUser("machine-1", "reader-1")
		require.NoError(t, err)
		assert.Nil(t, stored, "rejected grant must not create an access row")
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "no-such-machine", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionRead,
		})

		require.ErrorIs(t, err, errors.ErrMachineNotFound)
		assert.Nil(t, access)
	})

	t.Run("non-owner grantors are denied", func(t *testing.T) {
		cases := []struct {
			name       string
			grantorID  string
			permission string
		}{
			{"read-level grantee", "reader-1", models.PermissionRead},
			{"write-level grantee", "writer-1", models.PermissionWrite},
			{"user without any access row", "stranger-1", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				service, _, accessRepo := newAccessTestService()
				if tc.permission != "" {
					accessRepo.AddAccess("machine-1", tc.grantorID, tc.permission, time.Now().UTC())
				}

				access, err := service.GrantAccess(tc.grantorID, "machine-1", &GrantAccessRequest{
					UserID:     "friend-1",
					Permission: models.PermissionRead,
				})

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				assert.Nil(t, access)
				stored, err := accessRepo.GetByMachineAndUser("machine-1", "friend-1")
				require.NoError(t, err)
				assert.Nil(t, stored, "denied grant must not create an access row")
			})
		}
	})

	t.Run("duplicate grant is rejected", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now().UTC())

		access, err := service.GrantAccess("owner-1", "machine-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionWrite,
		})

		require.ErrorIs(t, err, errors.ErrAccessAlreadyGranted)
		assert.Nil(t, access)
	})
}

// TestMachineService_RevokeAccess covers the DELETE
// /api/machines/{id}/access/{userId} rules at the service seam:
// owner-only revoke, no self-revoke, over mock repositories.
func TestMachineService_RevokeAccess(t *testing.T) {
	t.Run("owner revokes a grantee and the row is gone", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now().UTC())

		err := service.RevokeAccess("owner-1", "machine-1", "reader-1")

		require.NoError(t, err)
		stored, err := accessRepo.GetByMachineAndUser("machine-1", "reader-1")
		require.NoError(t, err)
		assert.Nil(t, stored, "revoked access row must be gone")
	})

	t.Run("non-owner revokers are denied", func(t *testing.T) {
		cases := []struct {
			name       string
			revokerID  string
			permission string
		}{
			{"read-level grantee", "reader-1", models.PermissionRead},
			{"write-level grantee", "writer-1", models.PermissionWrite},
			{"user without any access row", "stranger-1", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				service, _, accessRepo := newAccessTestService()
				accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now().UTC())
				if tc.permission != "" {
					accessRepo.AddAccess("machine-1", tc.revokerID, tc.permission, time.Now().UTC())
				}

				err := service.RevokeAccess(tc.revokerID, "machine-1", "reader-1")

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				stored, err := accessRepo.GetByMachineAndUser("machine-1", "reader-1")
				require.NoError(t, err)
				assert.NotNil(t, stored, "denied revoke must not touch the access row")
			})
		}
	})

	t.Run("the owner's own access cannot be revoked", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		err := service.RevokeAccess("owner-1", "machine-1", "owner-1")

		require.ErrorIs(t, err, errors.ErrCannotRevokeOwner)
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		err := service.RevokeAccess("owner-1", "no-such-machine", "reader-1")

		require.ErrorIs(t, err, errors.ErrMachineNotFound)
	})
}

// TestMachineService_ListAccess covers the GET /api/machines/{id}/access
// rule at the service seam: the caller's access row is re-checked even
// though the subtree middleware already gated the route.
func TestMachineService_ListAccess(t *testing.T) {
	t.Run("a caller with an access row sees the machine's access list", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now().UTC())

		accesses, err := service.ListAccess("reader-1", "machine-1")

		require.NoError(t, err)
		require.Len(t, accesses, 2)
		byUser := make(map[string]string, len(accesses))
		for _, access := range accesses {
			byUser[access.UserID] = access.Permission
		}
		assert.Equal(t, models.PermissionAdmin, byUser["owner-1"])
		assert.Equal(t, models.PermissionRead, byUser["reader-1"])
	})

	t.Run("a caller without an access row is denied", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		accesses, err := service.ListAccess("stranger-1", "machine-1")

		require.ErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, accesses)
	})
}

// TestMachineService_AccessCheckPropagatesDBErrors pins the correctness
// fix behind the gated reads (inspect, status, access listing): a
// failing access lookup surfaces as an error (500 INTERNAL_ERROR via
// the envelope), never as the 403 of a silent denial.
func TestMachineService_AccessCheckPropagatesDBErrors(t *testing.T) {
	dbErr := fmt.Errorf("disk I/O error")

	// GetMachineByID decides access through the permission-level lookup
	// (GetPermission); ListAccess still probes row existence (HasAccess).
	newBrokenAccessService := func(broken func(*MachineService)) *MachineService {
		service, _, _ := newAccessTestService()
		broken(service)
		return service
	}

	t.Run("GetMachineByID surfaces the lookup failure", func(t *testing.T) {
		service := newBrokenAccessService(func(s *MachineService) {
			s.accessRepo = testutils.NewMockMachineAccessRepository().WithGetPermissionError(dbErr)
		})

		machine, err := service.GetMachineByID("owner-1", "machine-1")

		require.Error(t, err)
		require.ErrorIs(t, err, errors.ErrInternalError,
			"a failing level lookup answers INTERNAL_ERROR, never a silent denial")
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, machine)
	})

	t.Run("ListAccess surfaces the lookup failure", func(t *testing.T) {
		service := newBrokenAccessService(func(s *MachineService) {
			s.accessRepo = testutils.NewMockMachineAccessRepositoryWithErrors(nil, nil, nil, nil, nil, nil, dbErr)
		})

		accesses, err := service.ListAccess("owner-1", "machine-1")

		require.Error(t, err)
		require.ErrorIs(t, err, dbErr)
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, accesses)
	})

	t.Run("GetMachineStatus surfaces the lookup failure before probing", func(t *testing.T) {
		service := newBrokenAccessService(func(s *MachineService) {
			s.accessRepo = testutils.NewMockMachineAccessRepository().WithGetPermissionError(dbErr)
		})

		status, checkedAt, err := service.GetMachineStatus(context.Background(), "owner-1", "machine-1")

		require.Error(t, err)
		require.ErrorIs(t, err, errors.ErrInternalError)
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Empty(t, status)
		assert.True(t, checkedAt.IsZero(), "no probe must run when the access check fails")
	})
}
