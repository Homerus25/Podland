package machines

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errResolverMustNotRun fails the test loudly if the resolver seam runs
// where the policy must answer without resolution (IP literals, disabled
// policy).
var errResolverMustNotRun = errors.New("resolver must not run here")

// newHostPolicyService builds a create-path service over mock repos with
// the given host policy, so the wiring tests pin where the check runs
// (before the probe, before the persist) and what it never does (dial a
// blocked target).
func newHostPolicyService(policy HostPolicy) (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository, *testutils.MockConnectionManager) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManager()
	service := NewMachineService(machineRepo, accessRepo, connManager,
		testutils.NewMockEncryptionService(), newFakeSSHInvalidator()).
		WithHostPolicy(policy)
	return service, machineRepo, accessRepo, connManager
}

// newHostPolicyReq is a well-formed registration body whose hostname the
// subtest mutates.
func newHostPolicyReq(hostname string) *CreateMachineRequest {
	return &CreateMachineRequest{
		Name:             "test-machine",
		Hostname:         hostname,
		Port:             22,
		SSHUsername:      "root",
		SSHPrivateKey:    testutils.GenerateValidPEMKey(),
		PodmanSocketPath: "/run/podman/podman.sock",
	}
}

// TestMachineService_Create_HostPolicy pins the Create wiring of the
// opt-in host policy (pentest 2026-11-01 finding 1): the check runs right
// before the probe, a blocked host is never dialed and never persisted,
// and every other 400 keeps its precedence.
func TestMachineService_Create_HostPolicy(t *testing.T) {
	ctx := context.Background()
	userID := "test-user-123"

	t.Run("blocked literal hostname answers HOST_NOT_ALLOWED and never dials", func(t *testing.T) {
		// The resolver is poisoned: a literal must be checked directly.
		service, machineRepo, accessRepo, connManager := newHostPolicyService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)})

		machine, err := service.Create(ctx, userID, newHostPolicyReq("10.0.0.5"))

		require.Error(t, err)
		assert.Nil(t, machine)
		appErr := requireAppError(t, err, "HOST_NOT_ALLOWED", http.StatusBadRequest)
		assert.Equal(t, appErrors.ErrHostNotAllowed.Message, appErr.Message)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"a blocked host must never be dialed — the policy precedes the probe")
		machines, err := machineRepo.ListAccessible(userID)
		require.NoError(t, err)
		assert.Empty(t, machines, "a blocked host must never be persisted")
		access, err := accessRepo.GetByMachineAndUser("anything", userID)
		require.NoError(t, err)
		assert.Nil(t, access, "no owner access for a blocked host")
	})

	t.Run("hostname resolving to a blocked IP answers HOST_NOT_ALLOWED and never dials", func(t *testing.T) {
		service, _, _, connManager := newHostPolicyService(newTestPolicy([]netip.Addr{
			mustAddr(t, "93.184.216.34"),
			mustAddr(t, "169.254.169.254"),
		}, nil))

		_, err := service.Create(ctx, userID, newHostPolicyReq("metadata.example.com"))

		requireAppError(t, err, "HOST_NOT_ALLOWED", http.StatusBadRequest)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"a hostname resolving into a blocked range must never be dialed")
	})

	t.Run("public hostname passes the policy and probes as before", func(t *testing.T) {
		service, machineRepo, _, connManager := newHostPolicyService(newTestPolicy([]netip.Addr{
			mustAddr(t, "93.184.216.34"),
		}, nil))

		machine, err := service.Create(ctx, userID, newHostPolicyReq("example.com"))

		require.NoError(t, err)
		require.NotNil(t, machine)
		calls := connManager.TestConnectionCalls()
		require.Len(t, calls, 1, "a policy-passing host reaches the probe exactly once")
		assert.Equal(t, "example.com", calls[0].Hostname)
		saved, err := machineRepo.GetByID(machine.ID)
		require.NoError(t, err)
		require.NotNil(t, saved, "a policy-passing registration persists")
	})

	t.Run("disabled policy leaves registration unchanged — no resolution, no new failure modes", func(t *testing.T) {
		// No WithHostPolicy: the zero-value policy must behave exactly
		// as before. A loopback hostname (the primary use case) still
		// reaches the probe, and the resolver never runs.
		machineRepo := testutils.NewMockMachineRepository()
		accessRepo := testutils.NewMockMachineAccessRepository()
		connManager := testutils.NewMockConnectionManager()
		service := NewMachineService(machineRepo, accessRepo, connManager,
			testutils.NewMockEncryptionService(), newFakeSSHInvalidator())

		machine, err := service.Create(ctx, userID, newHostPolicyReq("127.0.0.1"))

		require.NoError(t, err)
		require.NotNil(t, machine)
		require.Len(t, connManager.TestConnectionCalls(), 1,
			"with the block off, a loopback host probes exactly as before")
	})

	t.Run("duplicate name keeps its precedence over the host policy", func(t *testing.T) {
		service, machineRepo, _, _ := newHostPolicyService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)})
		machineRepo.AddMachine(&models.Machine{ID: "existing", Name: "test-machine", OwnerID: userID})

		_, err := service.Create(ctx, userID, newHostPolicyReq("10.0.0.5"))

		requireAppError(t, err, "MACHINE_ALREADY_EXISTS", http.StatusBadRequest)
	})

	t.Run("missing fields keep their precedence over the host policy", func(t *testing.T) {
		service, _, _, _ := newHostPolicyService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)})

		req := newHostPolicyReq("10.0.0.5")
		req.SSHUsername = ""
		_, err := service.Create(ctx, userID, req)

		requireAppError(t, err, "VALIDATION_FAILED", http.StatusBadRequest)
	})

	t.Run("invalid PEM keeps its precedence over the host policy", func(t *testing.T) {
		service, _, _, _ := newHostPolicyService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)})

		req := newHostPolicyReq("10.0.0.5")
		req.SSHPrivateKey = testutils.GenerateInvalidPEMKey()
		_, err := service.Create(ctx, userID, req)

		requireAppError(t, err, "INVALID_SSH_KEY", http.StatusBadRequest)
	})
}

// TestMachineService_Update_HostPolicy pins the Update wiring: the
// policy applies whenever a hostname is set — with or without a key
// change — before the probe and before the persist, while the owner-only
// gate keeps its precedence.
func TestMachineService_Update_HostPolicy(t *testing.T) {
	ctx := context.Background()

	// newPolicyUpdateService builds the one-machine update world
	// (machine-1, owner owner-1, stored hostname old.example.com) over
	// the given policy and connection manager.
	newPolicyUpdateService := func(policy HostPolicy, connManager *testutils.MockConnectionManager) (*MachineService, *testutils.MockMachineRepository) {
		machineRepo := testutils.NewMockMachineRepository()
		accessRepo := testutils.NewMockMachineAccessRepository()
		service := NewMachineService(machineRepo, accessRepo, connManager,
			testutils.NewMockEncryptionService(), newFakeSSHInvalidator()).
			WithHostPolicy(policy)
		machineRepo.AddMachine(&models.Machine{
			ID:                "machine-1",
			Name:              "old-name",
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
		return service, machineRepo
	}

	t.Run("hostname change without a key is still policy-checked and never dialed", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, machineRepo := newPolicyUpdateService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)}, connManager)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Hostname: "192.168.1.10",
		})

		require.Error(t, err)
		assert.Nil(t, machine)
		requireAppError(t, err, "HOST_NOT_ALLOWED", http.StatusBadRequest)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"no key change: the probe would not run anyway — but the policy must still gate the hostname")
		assert.Empty(t, machineRepo.UpdateCalls(),
			"a blocked hostname must never be persisted")
	})

	t.Run("hostname change with a new key is policy-checked before the probe", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, machineRepo := newPolicyUpdateService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)}, connManager)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Hostname:      "10.255.0.1",
			SSHPrivateKey: testutils.GenerateValidPEMKey(),
		})

		require.Error(t, err)
		assert.Nil(t, machine)
		requireAppError(t, err, "HOST_NOT_ALLOWED", http.StatusBadRequest)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"the policy runs before the probe — a blocked host is never dialed, key change or not")
		assert.Empty(t, machineRepo.UpdateCalls())
		persisted, err := machineRepo.GetByID("machine-1")
		require.NoError(t, err)
		assert.Equal(t, "old-fingerprint", persisted.SSHKeyFingerprint,
			"stored credentials must be untouched")
	})

	t.Run("hostname resolving to a blocked IP is rejected on update", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, _ := newPolicyUpdateService(newTestPolicy([]netip.Addr{
			mustAddr(t, "169.254.169.254"),
		}, nil), connManager)

		_, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Hostname: "metadata.example.com",
		})

		requireAppError(t, err, "HOST_NOT_ALLOWED", http.StatusBadRequest)
		assert.Empty(t, connManager.TestConnectionCalls())
	})

	t.Run("owner-only gate keeps its precedence over the host policy", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, _ := newPolicyUpdateService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)}, connManager)

		_, err := service.Update(ctx, "writer-1", "machine-1", &CreateMachineRequest{
			Hostname: "10.0.0.5",
		})

		require.ErrorIs(t, err, appErrors.ErrAccessDenied,
			"the owner check precedes the policy — a non-owner is denied regardless of the hostname")
	})

	t.Run("public hostname change passes and persists", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, machineRepo := newPolicyUpdateService(newTestPolicy([]netip.Addr{
			mustAddr(t, "93.184.216.34"),
		}, nil), connManager)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Hostname: "new.example.com",
		})

		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, "new.example.com", machine.Hostname)
		persisted, err := machineRepo.GetByID("machine-1")
		require.NoError(t, err)
		assert.Equal(t, "new.example.com", persisted.Hostname)
	})

	t.Run("update without a hostname is not policy-checked", func(t *testing.T) {
		connManager := testutils.NewMockConnectionManager()
		service, _ := newPolicyUpdateService(
			HostPolicy{enabled: true, resolver: fakeResolver(nil, errResolverMustNotRun)}, connManager)

		machine, err := service.Update(ctx, "owner-1", "machine-1", &CreateMachineRequest{
			Name: "renamed",
		})

		require.NoError(t, err)
		require.NotNil(t, machine)
		assert.Equal(t, "renamed", machine.Name,
			"a non-hostname update passes the policy untouched")
	})
}
