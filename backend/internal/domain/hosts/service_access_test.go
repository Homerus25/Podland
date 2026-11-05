package hosts

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

// newAccessTestService builds a host service over mock repos with one
// host (id "host-1", owner "owner-1") and the owner's admin access
// row, mirroring production registration.
func newAccessTestService() (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	service := NewHostService(hostRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	hostRepo.AddHost(&models.Host{
		ID:                "host-1",
		Name:              "shared-host",
		Hostname:          "host.example.com",
		Port:              22,
		SSHUsername:       "user",
		SSHKeyEncrypted:   []byte("encrypted"),
		SSHKeyFingerprint: "fingerprint",
		PodmanSocketPath:  "/run/podman/podman.sock",
		OwnerID:           "owner-1",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})
	accessRepo.AddAccess("host-1", "owner-1", models.PermissionAdmin, time.Now().UTC())
	return service, hostRepo, accessRepo
}

// TestHostService_GrantAccess covers the POST /api/hosts/{id}/access
// rules at the service seam: owner-only grant over mock repositories.
func TestHostService_GrantAccess(t *testing.T) {
	t.Run("owner grants a new user and the row persists", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "host-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionRead,
		})

		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Equal(t, "host-1", access.HostID)
		assert.Equal(t, "reader-1", access.UserID)
		assert.Equal(t, models.PermissionRead, access.Permission)
		assert.False(t, access.GrantedAt.IsZero())

		stored, err := accessRepo.GetByHostAndUser("host-1", "reader-1")
		require.NoError(t, err)
		require.NotNil(t, stored, "granted access row must persist")
		assert.Equal(t, models.PermissionRead, stored.Permission)
	})

	t.Run("invalid permission is rejected", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "host-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: "superuser",
		})

		var appErr *errors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "INVALID_PERMISSION", appErr.Code)
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
		assert.Nil(t, access)
		stored, err := accessRepo.GetByHostAndUser("host-1", "reader-1")
		require.NoError(t, err)
		assert.Nil(t, stored, "rejected grant must not create an access row")
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		access, err := service.GrantAccess("owner-1", "no-such-host", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionRead,
		})

		require.ErrorIs(t, err, errors.ErrHostNotFound)
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
					accessRepo.AddAccess("host-1", tc.grantorID, tc.permission, time.Now().UTC())
				}

				access, err := service.GrantAccess(tc.grantorID, "host-1", &GrantAccessRequest{
					UserID:     "friend-1",
					Permission: models.PermissionRead,
				})

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				assert.Nil(t, access)
				stored, err := accessRepo.GetByHostAndUser("host-1", "friend-1")
				require.NoError(t, err)
				assert.Nil(t, stored, "denied grant must not create an access row")
			})
		}
	})

	t.Run("duplicate grant is rejected", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now().UTC())

		access, err := service.GrantAccess("owner-1", "host-1", &GrantAccessRequest{
			UserID:     "reader-1",
			Permission: models.PermissionWrite,
		})

		require.ErrorIs(t, err, errors.ErrAccessAlreadyGranted)
		assert.Nil(t, access)
	})
}

// TestHostService_RevokeAccess covers the DELETE
// /api/hosts/{id}/access/{userId} rules at the service seam:
// owner-only revoke, no self-revoke, over mock repositories.
func TestHostService_RevokeAccess(t *testing.T) {
	t.Run("owner revokes a grantee and the row is gone", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now().UTC())

		err := service.RevokeAccess("owner-1", "host-1", "reader-1")

		require.NoError(t, err)
		stored, err := accessRepo.GetByHostAndUser("host-1", "reader-1")
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
				accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now().UTC())
				if tc.permission != "" {
					accessRepo.AddAccess("host-1", tc.revokerID, tc.permission, time.Now().UTC())
				}

				err := service.RevokeAccess(tc.revokerID, "host-1", "reader-1")

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				stored, err := accessRepo.GetByHostAndUser("host-1", "reader-1")
				require.NoError(t, err)
				assert.NotNil(t, stored, "denied revoke must not touch the access row")
			})
		}
	})

	t.Run("the owner's own access cannot be revoked", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		err := service.RevokeAccess("owner-1", "host-1", "owner-1")

		require.ErrorIs(t, err, errors.ErrCannotRevokeOwner)
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		service, _, _ := newAccessTestService()

		err := service.RevokeAccess("owner-1", "no-such-host", "reader-1")

		require.ErrorIs(t, err, errors.ErrHostNotFound)
	})
}

// TestHostService_ListAccess covers the GET /api/hosts/{id}/access
// rule at the service seam: the caller's access row is re-checked even
// though the subtree middleware already gated the route.
func TestHostService_ListAccess(t *testing.T) {
	t.Run("a caller with an access row sees the host's access list", func(t *testing.T) {
		service, _, accessRepo := newAccessTestService()
		accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now().UTC())

		accesses, err := service.ListAccess("reader-1", "host-1")

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

		accesses, err := service.ListAccess("stranger-1", "host-1")

		require.ErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, accesses)
	})
}

// TestHostService_AccessCheckPropagatesDBErrors pins the correctness
// fix behind the gated reads (inspect, status, access listing): a
// failing access lookup surfaces as an error (500 INTERNAL_ERROR via
// the envelope), never as the 403 of a silent denial.
func TestHostService_AccessCheckPropagatesDBErrors(t *testing.T) {
	dbErr := fmt.Errorf("disk I/O error")

	// GetHostByID decides access through the permission-level lookup
	// (GetPermission); ListAccess still probes row existence (HasAccess).
	newBrokenAccessService := func(broken func(*HostService)) *HostService {
		service, _, _ := newAccessTestService()
		broken(service)
		return service
	}

	t.Run("GetHostByID surfaces the lookup failure", func(t *testing.T) {
		service := newBrokenAccessService(func(s *HostService) {
			s.accessRepo = testutils.NewMockHostAccessRepository().WithGetPermissionError(dbErr)
		})

		host, err := service.GetHostByID("owner-1", "host-1")

		require.Error(t, err)
		require.ErrorIs(t, err, errors.ErrInternalError,
			"a failing level lookup answers INTERNAL_ERROR, never a silent denial")
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, host)
	})

	t.Run("ListAccess surfaces the lookup failure", func(t *testing.T) {
		service := newBrokenAccessService(func(s *HostService) {
			s.accessRepo = testutils.NewMockHostAccessRepositoryWithErrors(nil, nil, nil, nil, nil, nil, dbErr)
		})

		accesses, err := service.ListAccess("owner-1", "host-1")

		require.Error(t, err)
		require.ErrorIs(t, err, dbErr)
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Nil(t, accesses)
	})

	t.Run("GetHostStatus surfaces the lookup failure before probing", func(t *testing.T) {
		service := newBrokenAccessService(func(s *HostService) {
			s.accessRepo = testutils.NewMockHostAccessRepository().WithGetPermissionError(dbErr)
		})

		status, checkedAt, err := service.GetHostStatus(context.Background(), "owner-1", "host-1")

		require.Error(t, err)
		require.ErrorIs(t, err, errors.ErrInternalError)
		require.NotErrorIs(t, err, errors.ErrAccessDenied)
		assert.Empty(t, status)
		assert.True(t, checkedAt.IsZero(), "no probe must run when the access check fails")
	})
}
