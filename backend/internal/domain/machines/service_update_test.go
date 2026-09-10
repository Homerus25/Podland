package machines

import (
	"context"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUpdateTestService builds a machine service over mock repos with one
// machine (id "machine-1", owner "owner-1"). A non-empty permission grants
// the caller an access row at that level; empty permission leaves the
// caller without any access row.
func newUpdateTestService(callerID, permission string) (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineService(machineRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService())
	machineRepo.AddMachine(&models.Machine{
		ID:                "machine-1",
		Name:              "old-name",
		Description:       "old description",
		Hostname:          "old.example.com",
		Port:              22,
		SSHUsername:       "old-user",
		SSHKeyEncrypted:   []byte("old-encrypted"),
		SSHKeyFingerprint: "old-fingerprint",
		PodmanSocketPath:  "/run/podman/podman.sock",
		OwnerID:           "owner-1",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})
	if permission != "" {
		accessRepo.AddAccess("machine-1", callerID, permission, time.Now().UTC())
	}
	return service, machineRepo, accessRepo
}

// TestMachineService_Update covers the PUT /api/machines/{id} rules at the
// service seam: owner-only update over mock repositories.
func TestMachineService_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("owner updates fields and the write persists", func(t *testing.T) {
		service, machineRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Name:        "new-name",
			Hostname:    "new.example.com",
			Port:        2222,
			SSHUsername: "new-user",
		})

		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, "new-name", machine.Name)
		assert.Equal(t, "new.example.com", machine.Hostname)
		assert.Equal(t, 2222, machine.Port)
		assert.Equal(t, "new-user", machine.SSHUsername)

		persisted, err := machineRepo.GetByID("machine-1")
		require.NoError(t, err)
		require.NotNil(t, persisted)
		assert.Equal(t, "new-name", persisted.Name)
		assert.Equal(t, "new.example.com", persisted.Hostname)
		assert.Equal(t, 2222, persisted.Port)
		assert.Equal(t, "new-user", persisted.SSHUsername)
	})

	t.Run("non-owners are denied", func(t *testing.T) {
		cases := []struct {
			name       string
			callerID   string
			permission string
		}{
			{"read-level grantee", "reader-1", models.PermissionRead},
			{"write-level grantee", "writer-1", models.PermissionWrite},
			{"user without any access row", "stranger-1", ""},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				service, machineRepo, _ := newUpdateTestService(tc.callerID, tc.permission)

				machine, err := service.Update(ctx, tc.callerID, "machine-1", &CreateMachineRequest{Name: "hijacked"})

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				assert.Nil(t, machine)
				persisted, err := machineRepo.GetByID("machine-1")
				require.NoError(t, err)
				assert.Equal(t, "old-name", persisted.Name, "denied update must not touch stored fields")
			})
		}
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		service, _, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		machine, err := service.Update(ctx, "owner-1", "no-such-machine", &CreateMachineRequest{Name: "ghost"})

		require.ErrorIs(t, err, errors.ErrMachineNotFound)
		assert.Nil(t, machine)
	})

	t.Run("invalid PEM key is rejected", func(t *testing.T) {
		service, machineRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			SSHPrivateKey: testutils.GenerateInvalidPEMKey(),
		})

		require.ErrorIs(t, err, errors.ErrInvalidSSHKey)
		assert.Nil(t, machine)
		persisted, err := machineRepo.GetByID("machine-1")
		require.NoError(t, err)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint, "rejected key must not touch stored credentials")
	})

	t.Run("empty request fields keep stored values", func(t *testing.T) {
		service, _, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Name:        "only-name",
			Description: "only description",
		})

		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, "only-name", machine.Name)
		assert.Equal(t, "only description", machine.Description)
		assert.Equal(t, "old.example.com", machine.Hostname)
		assert.Equal(t, 22, machine.Port)
		assert.Equal(t, "old-user", machine.SSHUsername)
		assert.Equal(t, "old-fingerprint", machine.SSHKeyFingerprint)
		assert.Equal(t, "/run/podman/podman.sock", machine.PodmanSocketPath)
		assert.Equal(t, "old-encrypted", string(machine.SSHKeyEncrypted), "no new key: stored encrypted key stays")
	})

	t.Run("new key is encrypted and fingerprinted before the write", func(t *testing.T) {
		service, machineRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)
		newKey := testutils.GenerateValidPEMKey()

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			SSHPrivateKey: newKey,
		})

		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, []byte("ssh-encrypted:"+newKey), machine.SSHKeyEncrypted)
		assert.NotEqual(t, "old-fingerprint", machine.SSHKeyFingerprint)
		assert.NotContains(t, string(machine.SSHKeyEncrypted), "old-encrypted")

		persisted, err := machineRepo.GetByID("machine-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("ssh-encrypted:"+newKey), persisted.SSHKeyEncrypted)
		assert.Equal(t, machine.SSHKeyFingerprint, persisted.SSHKeyFingerprint)
	})
}
