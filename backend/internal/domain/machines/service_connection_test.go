package machines

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

// TestMachineService_Create_ConnectionTest covers the synchronous
// connection probe added to MachineService.Create by issue 080-05
// (description_backend.md §Machine Registration step 5). The probe runs
// BEFORE the machine is persisted, so a bad host / bad credentials / wrong
// socket path fails fast at registration (400 CONNECTION_FAILED) instead
// of silently registering an unreachable machine that only fails at first
// use.
func TestMachineService_Create_ConnectionTest(t *testing.T) {
	ctx := context.Background()
	userID := "test-user-123"

	// validReq is a template with a well-formed PEM key; subtests mutate
	// the relevant field. Port and socket are set explicitly where the
	// subtest cares about what the probe received; the "defaulting"
	// subtest leaves them zero to exercise Create's defaults.
	newReq := func() *CreateMachineRequest {
		return &CreateMachineRequest{
			Name:             "test-machine",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}
	}

	// --- AC: probe failure -> 400 CONNECTION_FAILED, machine NOT persisted -
	t.Run("connection test failure returns ErrConnectionTestFailed and does not persist", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		// Any non-nil error stands in for "SSH/podman dial failed"
		// (bad host / bad credentials / wrong socket all surface here).
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("dial tcp: connection refused"))
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)

		machine, err := service.Create(ctx, userID, newReq())

		// The registration must surface a 400 CONNECTION_FAILED — the
		// exact sentinel the handler type-asserts to pick up its Status.
		require.Error(t, err)
		assert.Nil(t, machine, "no machine should be returned when the probe fails")
		assert.Equal(t, appErrors.ErrConnectionTestFailed, err,
			"probe failure must map to ErrConnectionTestFailed (400 CONNECTION_FAILED)")

		// "Not persisted" is observable through the repository's public
		// API: no machine row for this owner, and no access grant.
		owned, err := mockMachineRepo.ListByOwner(userID)
		require.NoError(t, err)
		assert.Empty(t, owned, "machine must NOT be persisted when the probe fails")
		access, err := mockAccessRepo.GetByMachineAndUser("anything", userID)
		require.NoError(t, err)
		assert.Nil(t, access, "owner access must NOT be granted when the probe fails")
	})

	// --- AC: probe timeout -> 400 CONNECTION_TEST_TIMEOUT -------------
	//
	// The connection manager signals a probe timeout by returning
	// context.DeadlineExceeded (see ConnectionManager.TestConnection).
	// Create must surface this as ErrConnectionTestTimeout — distinct
	// from CONNECTION_FAILED so a client can tell "unreachable" from
	// "slow" — and still must not persist the machine.
	t.Run("connection test timeout returns ErrConnectionTestTimeout and does not persist", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(context.DeadlineExceeded)
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)

		machine, err := service.Create(ctx, userID, newReq())

		require.Error(t, err)
		assert.Nil(t, machine)
		assert.Equal(t, appErrors.ErrConnectionTestTimeout, err,
			"probe timeout must map to ErrConnectionTestTimeout (400)")
		owned, err := mockMachineRepo.ListByOwner(userID)
		require.NoError(t, err)
		assert.Empty(t, owned, "machine must NOT be persisted when the probe times out")
	})

	// --- AC: probe receives the defaulted port and socket ------------
	//
	// Create applies port/socket defaults before probing, so a request
	// that omits them is still tested against the real port (22) and
	// socket (/run/podman/podman.sock) the machine will use at runtime.
	// Asserting the mock saw those defaults proves the probe is not
	// dialing :0 / "" (which would always fail) — a regression that a
	// bare "probe was called" check would miss.
	t.Run("probe receives defaulted port and socket when request omits them", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		req := newReq()
		req.Port = 0              // expect default 22 in the probe
		req.PodmanSocketPath = "" // expect default /run/podman/podman.sock

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)
		machine, err := service.Create(ctx, userID, req)
		require.NoError(t, err)
		require.NotNil(t, machine)

		calls := mockConnManager.TestConnectionCalls()
		require.Len(t, calls, 1, "Create must invoke the probe exactly once")
		assert.Equal(t, "example.com", calls[0].Hostname)
		assert.Equal(t, 22, calls[0].Port, "probe must receive the defaulted port")
		assert.Equal(t, "root", calls[0].SSHUsername)
		assert.Equal(t, "/run/podman/podman.sock", calls[0].PodmanSocketPath,
			"probe must receive the defaulted socket path")
		assert.Equal(t, req.SSHPrivateKey, calls[0].SSHPrivateKey,
			"probe must receive the plaintext key (the documented flow runs before encryption)")
	})

	// --- AC: happy path probes with the request's real credentials ---
	//
	// A reachable target registers (201 path): the probe runs, returns
	// nil, and the machine is persisted. This mirrors the integration
	// suite's good-machine registration but at the unit level — proving
	// the probe is wired into the success path, not just the failure
	// path. The default mock returns nil (success), so this also guards
	// against a regression that skips the probe on the happy path.
	t.Run("reachable target registers and probe sees the supplied credentials", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		req := newReq() // port 22, explicit socket
		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)

		machine, err := service.Create(ctx, userID, req)
		require.NoError(t, err)
		require.NotNil(t, machine)

		// Probe ran with the exact credentials on the request.
		calls := mockConnManager.TestConnectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "example.com", calls[0].Hostname)
		assert.Equal(t, 22, calls[0].Port)
		assert.Equal(t, "root", calls[0].SSHUsername)
		assert.Equal(t, "/run/podman/podman.sock", calls[0].PodmanSocketPath)

		// Machine was persisted (not blocked by the probe).
		saved, err := mockMachineRepo.GetByID(machine.ID)
		require.NoError(t, err)
		require.NotNil(t, saved)
		assert.Equal(t, machine.ID, saved.ID)
	})

	// --- Ordering: name-uniqueness runs BEFORE the probe -------------
	//
	// A duplicate name should be rejected without paying the cost of an
	// SSH dial. This pins the documented order (validate -> name unique
	// -> PEM -> connection test) and guards a regression that probes
	// first, which would make a dup-name request needlessly slow and
	// would mask the MACHINE_ALREADY_EXISTS code behind a probe failure.
	t.Run("duplicate name is rejected without invoking the connection probe", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("probe should not have been called"))
		mockEncryption := testutils.NewMockEncryptionService()

		// Seed an existing machine with the same name for this owner.
		mockMachineRepo.AddMachine(&models.Machine{
			ID:      "existing-machine",
			Name:    "test-machine",
			OwnerID: userID,
		})

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)
		machine, err := service.Create(ctx, userID, newReq())

		require.Error(t, err)
		assert.Nil(t, machine)
		assert.Equal(t, appErrors.ErrMachineAlreadyExists, err)
		assert.Empty(t, mockConnManager.TestConnectionCalls(),
			"the probe must NOT run when the name is a duplicate — name uniqueness precedes the probe")
	})

	// --- Ordering: probe runs BEFORE encryption ----------------------
	//
	// The documented flow runs the probe against the plaintext key and
	// only encrypts on success. A regression that encrypts first would
	// waste CPU on a doomed registration and (worse) would couple the
	// probe to the encrypt/decrypt round-trip. Assert encryption is not
	// reached when the probe fails.
	t.Run("failed probe does not encrypt the SSH key", func(t *testing.T) {
		mockMachineRepo := testutils.NewMockMachineRepository()
		mockAccessRepo := testutils.NewMockMachineAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("dial tcp: connection refused"))

		encryptCalled := false
		mockEncryption := testutils.NewMockEncryptionServiceWithCustomFunctions(
			nil, nil,
			func(string) ([]byte, string, error) {
				encryptCalled = true
				return []byte("x"), "fp", nil
			},
			nil,
		)

		service := NewMachineService(mockMachineRepo, mockAccessRepo, mockConnManager, mockEncryption)
		_, err := service.Create(ctx, userID, newReq())
		require.Error(t, err)

		assert.False(t, encryptCalled,
			"the SSH key must NOT be encrypted when the probe fails — the probe precedes encryption")
	})
}
