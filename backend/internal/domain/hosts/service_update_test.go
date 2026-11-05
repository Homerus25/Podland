package hosts

import (
	"context"
	stderrors "errors"
	"net/http"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUpdateTestService builds a host service over mock repos with one
// host (id "host-1", owner "owner-1"). A non-empty permission grants
// the caller an access row at that level; empty permission leaves the
// caller without any access row.
func newUpdateTestService(callerID, permission string) (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	return newUpdateTestServiceWithConn(testutils.NewMockConnectionManager(), callerID, permission)
}

// newUpdateTestServiceWithConn builds the same one-host world over a
// caller-supplied connection manager, so probe tests can inject probe
// failures and inspect the recorded probe calls.
func newUpdateTestServiceWithConn(conn *testutils.MockConnectionManager, callerID, permission string) (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	service := NewHostService(hostRepo, accessRepo, conn, testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	hostRepo.AddHost(&models.Host{
		ID:                "host-1",
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
		accessRepo.AddAccess("host-1", callerID, permission, time.Now().UTC())
	}
	return service, hostRepo, accessRepo
}

// TestHostService_Update covers the PUT /api/hosts/{id} rules at the
// service seam: owner-only update over mock repositories.
func TestHostService_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("owner updates fields and the write persists", func(t *testing.T) {
		service, hostRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			Name:        "new-name",
			Hostname:    "new.example.com",
			Port:        2222,
			SSHUsername: "new-user",
		})

		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Equal(t, "new-name", host.Name)
		assert.Equal(t, "new.example.com", host.Hostname)
		assert.Equal(t, 2222, host.Port)
		assert.Equal(t, "new-user", host.SSHUsername)

		persisted, err := hostRepo.GetByID("host-1")
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
				service, hostRepo, _ := newUpdateTestService(tc.callerID, tc.permission)

				host, err := service.Update(ctx, tc.callerID, "host-1", &CreateHostRequest{Name: "hijacked"})

				require.ErrorIs(t, err, errors.ErrAccessDenied)
				assert.Nil(t, host)
				persisted, err := hostRepo.GetByID("host-1")
				require.NoError(t, err)
				assert.Equal(t, "old-name", persisted.Name, "denied update must not touch stored fields")
			})
		}
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		service, _, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "no-such-host", &CreateHostRequest{Name: "ghost"})

		require.ErrorIs(t, err, errors.ErrHostNotFound)
		assert.Nil(t, host)
	})

	t.Run("invalid PEM key is rejected", func(t *testing.T) {
		service, hostRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			SSHPrivateKey: testutils.GenerateInvalidPEMKey(),
		})

		require.Error(t, err)
		assert.Nil(t, host)
		var appErr *errors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "INVALID_SSH_KEY", appErr.Code)
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
		assert.Contains(t, appErr.Message, "PRIVATE KEY",
			"the update path shares the enriched PEM-format message (issue 399)")
		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint, "rejected key must not touch stored credentials")
	})

	t.Run("empty request fields keep stored values", func(t *testing.T) {
		service, _, _ := newUpdateTestService("owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			Name:        "only-name",
			Description: "only description",
		})

		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Equal(t, "only-name", host.Name)
		assert.Equal(t, "only description", host.Description)
		assert.Equal(t, "old.example.com", host.Hostname)
		assert.Equal(t, 22, host.Port)
		assert.Equal(t, "old-user", host.SSHUsername)
		assert.Equal(t, "old-fingerprint", host.SSHKeyFingerprint)
		assert.Equal(t, "/run/podman/podman.sock", host.PodmanSocketPath)
		assert.Equal(t, "old-encrypted", string(host.SSHKeyEncrypted), "no new key: stored encrypted key stays")
	})

	t.Run("new key is encrypted and fingerprinted before the write", func(t *testing.T) {
		service, hostRepo, _ := newUpdateTestService("owner-1", models.PermissionAdmin)
		newKey := testutils.GenerateValidPEMKey()

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			SSHPrivateKey: newKey,
		})

		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Equal(t, []byte("ssh-encrypted:"+newKey), host.SSHKeyEncrypted)
		assert.NotEqual(t, "old-fingerprint", host.SSHKeyFingerprint)
		assert.NotContains(t, string(host.SSHKeyEncrypted), "old-encrypted")

		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("ssh-encrypted:"+newKey), persisted.SSHKeyEncrypted)
		assert.Equal(t, host.SSHKeyFingerprint, persisted.SSHKeyFingerprint)
	})
}

// TestHostService_Update_ConnectionProbe covers the registration-style
// connection probe on key change: an owner providing a new SSH private key
// must have the merged connection configuration probed BEFORE anything is
// persisted — a broken target is rejected (400) instead of silently saving
// credentials that only fail at first use. Updates without a key change
// skip the probe entirely.
func TestHostService_Update_ConnectionProbe(t *testing.T) {
	ctx := context.Background()

	t.Run("new key probes the merged configuration and the update persists", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, hostRepo, _ := newUpdateTestServiceWithConn(connManager, "owner-1", models.PermissionAdmin)
		newKey := testutils.GenerateValidPEMKey()

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			Name:             "renamed",
			Hostname:         "new.example.com",
			Port:             2222,
			SSHUsername:      "new-user",
			SSHPrivateKey:    newKey,
			PodmanSocketPath: "/custom/podman.sock",
		})

		require.NoError(t, err)
		require.NotNil(t, host)

		calls := connManager.TestConnectionCalls()
		require.Len(t, calls, 1, "a key change must invoke the probe exactly once")
		assert.Equal(t, "new.example.com", calls[0].Hostname)
		assert.Equal(t, 2222, calls[0].Port)
		assert.Equal(t, "new-user", calls[0].SSHUsername)
		assert.Equal(t, newKey, calls[0].SSHPrivateKey,
			"probe must receive the plaintext new key (registration style: probe before encryption)")
		assert.Equal(t, "/custom/podman.sock", calls[0].PodmanSocketPath)

		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("ssh-encrypted:"+newKey), persisted.SSHKeyEncrypted,
			"a passing probe lets the update persist the new key")
	})

	t.Run("new key with only stored connection fields probes the merged target", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, _, _ := newUpdateTestServiceWithConn(connManager, "owner-1", models.PermissionAdmin)

		_, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		})
		require.NoError(t, err)

		calls := connManager.TestConnectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "old.example.com", calls[0].Hostname, "untouched fields must come from the stored host")
		assert.Equal(t, 22, calls[0].Port)
		assert.Equal(t, "old-user", calls[0].SSHUsername)
		assert.Equal(t, "/run/podman/podman.sock", calls[0].PodmanSocketPath)
	})

	t.Run("failed probe answers CONNECTION_FAILED and persists nothing", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManagerWithTestError(
			stderrors.New("dial tcp: connection refused"))
		service, hostRepo, _ := newUpdateTestServiceWithConn(connManager, "owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			Name:          "renamed-anyway",
			Hostname:      "new.example.com",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		})

		require.Error(t, err)
		assert.Nil(t, host)
		var appErr *errors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "CONNECTION_FAILED", appErr.Code,
			"probe failure keeps the CONNECTION_FAILED code")
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
		assert.Equal(t, "Failed to connect to host during registration — check the hostname, port, SSH username, private key, and podman socket path",
			appErr.Message,
			"update shares the registration probe path, so the static message rides for free (pentest finding 1)")

		// Nothing persisted: the repository write never happened, and the
		// stored credentials (written only after a passing probe) are
		// untouched.
		assert.Empty(t, hostRepo.UpdateCalls(),
			"a failed probe must persist nothing — no repository write at all")
		calls := connManager.TestConnectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "new.example.com", calls[0].Hostname,
			"the failing probe still targeted the merged configuration")
		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		require.NotNil(t, persisted)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint,
			"failed probe must not touch stored credentials")
		assert.Equal(t, "old-encrypted", string(persisted.SSHKeyEncrypted))
	})

	t.Run("probe timeout answers CONNECTION_TEST_TIMEOUT naming the bound and persists nothing", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManagerWithTestError(
			&connections.TestTimeoutError{Bound: 10 * time.Second})
		service, hostRepo, _ := newUpdateTestServiceWithConn(connManager, "owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		})

		require.Error(t, err)
		assert.Nil(t, host)
		var appErr *errors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, "CONNECTION_TEST_TIMEOUT", appErr.Code,
			"probe timeout must stay distinct from CONNECTION_FAILED")
		assert.Equal(t, http.StatusBadRequest, appErr.Status)
		assert.Equal(t, "Connection test timed out after 10s", appErr.Message,
			"the message must state the timeout bound the probe exceeded (issue 399)")

		assert.Empty(t, hostRepo.UpdateCalls(),
			"a timed-out probe must persist nothing — no repository write at all")
		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint,
			"timed-out probe must not persist the new key")
	})

	t.Run("update without a key change skips the probe entirely", func(t *testing.T) {
		// The manager is poisoned with a probe error: if the update ran
		// the probe despite no key change, the update itself would fail
		// — so NoError plus zero recorded calls proves the skip.
		connManager := testutils.NewMockConnectionManagerWithTestError(
			stderrors.New("probe must not run without a key change"))
		service, hostRepo, _ := newUpdateTestServiceWithConn(connManager, "owner-1", models.PermissionAdmin)

		host, err := service.Update(ctx, "owner-1", "host-1", &CreateHostRequest{
			Name:     "renamed",
			Hostname: "new.example.com",
			Port:     2222,
		})

		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"no key change: the probe must not run at all")

		persisted, err := hostRepo.GetByID("host-1")
		require.NoError(t, err)
		assert.Equal(t, "renamed", persisted.Name, "non-key updates still persist")
		assert.Equal(t, "new.example.com", persisted.Hostname)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint)
		assert.Equal(t, []string{"host-1"}, hostRepo.UpdateCalls(),
			"the non-key update still writes the host once")
	})
}
