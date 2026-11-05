package hosts

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/slogtest"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireAppError asserts the error is an AppError with the given code and
// HTTP status, returning it so the caller can pin the message — the
// enriched messages are WithMessage copies, so sentinel pointer equality
// no longer holds (issue 399).
func requireAppError(t *testing.T, err error, code string, status int) *appErrors.AppError {
	t.Helper()
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "the service must answer an AppError")
	assert.Equal(t, code, appErr.Code, "error code on the wire must stay stable")
	assert.Equal(t, status, appErr.Status, "the HTTP status belongs to the code")
	return appErr
}

// TestHostService_Create_ConnectionTest covers the synchronous
// connection probe added to HostService.Create by issue 080-05
// (description_backend.md §Host Registration step 5). The probe runs
// BEFORE the host is persisted, so a bad host / bad credentials / wrong
// socket path fails fast at registration (400 CONNECTION_FAILED) instead
// of silently registering an unreachable host that only fails at first
// use.
func TestHostService_Create_ConnectionTest(t *testing.T) {
	ctx := context.Background()
	userID := "test-user-123"

	// validReq is a template with a well-formed PEM key; subtests mutate
	// the relevant field. Port and socket are set explicitly where the
	// subtest cares about what the probe received; the "defaulting"
	// subtest leaves them zero to exercise Create's defaults.
	newReq := func() *CreateHostRequest {
		return &CreateHostRequest{
			Name:             "test-host",
			Hostname:         "example.com",
			Port:             22,
			SSHUsername:      "root",
			SSHPrivateKey:    testutils.GenerateValidPEMKey(),
			PodmanSocketPath: "/run/podman/podman.sock",
		}
	}

	// --- AC: probe failure -> 400 CONNECTION_FAILED with a static
	// message (pentest 2026-11-01 finding 1) ---
	t.Run("connection test failure answers 400 CONNECTION_FAILED with the static message", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		// Any non-nil error stands in for "SSH/podman dial failed"
		// (bad host / bad credentials / wrong socket all surface here).
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("dial tcp: connection refused"))
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		host, err := service.Create(ctx, userID, newReq())

		// The registration must surface a 400 CONNECTION_FAILED whose
		// message is STATIC (pentest finding 1): the raw dial error is
		// an internal port-scan / SSRF oracle (open vs closed vs
		// filtered, target address leaked), so no probe detail may
		// reach the client — the full cause lives in the server-side
		// log line.
		require.Error(t, err)
		assert.Nil(t, host, "no host should be returned when the probe fails")
		appErr := requireAppError(t, err, "CONNECTION_FAILED", http.StatusBadRequest)
		assert.Equal(t, "Failed to connect to host during registration — check the hostname, port, SSH username, private key, and podman socket path",
			appErr.Message, "the message must be the static text — no dial error, no target address")
		assert.NotContains(t, appErr.Message, "connection refused",
			"the underlying dial error must not leak into the client-visible message")

		// "Not persisted" is observable through the repository's public
		// API: no host row for this owner, and no access grant.
		hosts, err := mockHostRepo.ListAccessible(userID)
		require.NoError(t, err)
		assert.Empty(t, hosts, "host must NOT be persisted when the probe fails")
		access, err := mockAccessRepo.GetByHostAndUser("anything", userID)
		require.NoError(t, err)
		assert.Nil(t, access, "owner access must NOT be granted when the probe fails")
	})

	// --- AC: probe timeout -> 400 CONNECTION_TEST_TIMEOUT naming the bound
	// ---
	//
	// The connection manager signals a probe timeout with its typed
	// TestTimeoutError carrying the configured bound (see
	// ConnectionManager.TestConnection). Create must surface this as
	// ErrConnectionTestTimeout — distinct from CONNECTION_FAILED so a
	// client can tell "unreachable" from "slow" — with the actual bound
	// in the message, and still must not persist the host.
	t.Run("connection test timeout answers 400 CONNECTION_TEST_TIMEOUT naming the exceeded bound", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			&connections.TestTimeoutError{Bound: 10 * time.Second})
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		host, err := service.Create(ctx, userID, newReq())

		require.Error(t, err)
		assert.Nil(t, host)
		appErr := requireAppError(t, err, "CONNECTION_TEST_TIMEOUT", http.StatusBadRequest)
		assert.Equal(t, "Connection test timed out after 10s", appErr.Message,
			"the message must state the timeout bound the probe exceeded")
		hosts, err := mockHostRepo.ListAccessible(userID)
		require.NoError(t, err)
		assert.Empty(t, hosts, "host must NOT be persisted when the probe times out")
	})

	// --- Edge: the caller's own context deadline has no bound to name --
	//
	// TestConnection returns the caller's ctx.Err() when the request
	// context expires first (client disconnect / server deadline). That
	// is still a probe that did not finish, so it maps to the plain
	// timeout sentinel — without a configured bound there is nothing to
	// name in the message.
	t.Run("caller context deadline maps to the plain timeout sentinel", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(context.DeadlineExceeded)
		mockEncryption := testutils.NewMockEncryptionService()

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		_, err := service.Create(ctx, userID, newReq())

		require.Error(t, err)
		appErr := requireAppError(t, err, "CONNECTION_TEST_TIMEOUT", http.StatusBadRequest)
		assert.Equal(t, appErrors.ErrConnectionTestTimeout.Message, appErr.Message,
			"a caller-side deadline carries no configured bound: the plain sentinel message stays")
	})

	// --- AC: probe receives the defaulted port and socket ------------
	//
	// Create applies port/socket defaults before probing, so a request
	// that omits them is still tested against the real port (22) and
	// socket (/run/podman/podman.sock) the host will use at runtime.
	// Asserting the mock saw those defaults proves the probe is not
	// dialing :0 / "" (which would always fail) — a regression that a
	// bare "probe was called" check would miss.
	t.Run("probe receives defaulted port and socket when request omits them", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		req := newReq()
		req.Port = 0              // expect default 22 in the probe
		req.PodmanSocketPath = "" // expect default /run/podman/podman.sock

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())
		host, err := service.Create(ctx, userID, req)
		require.NoError(t, err)
		require.NotNil(t, host)

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
	// nil, and the host is persisted. This mirrors the integration
	// suite's good-host registration but at the unit level — proving
	// the probe is wired into the success path, not just the failure
	// path. The default mock returns nil (success), so this also guards
	// against a regression that skips the probe on the happy path.
	t.Run("reachable target registers and probe sees the supplied credentials", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManager()
		mockEncryption := testutils.NewMockEncryptionService()

		req := newReq() // port 22, explicit socket
		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())

		host, err := service.Create(ctx, userID, req)
		require.NoError(t, err)
		require.NotNil(t, host)

		// Probe ran with the exact credentials on the request.
		calls := mockConnManager.TestConnectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "example.com", calls[0].Hostname)
		assert.Equal(t, 22, calls[0].Port)
		assert.Equal(t, "root", calls[0].SSHUsername)
		assert.Equal(t, "/run/podman/podman.sock", calls[0].PodmanSocketPath)

		// Host was persisted (not blocked by the probe).
		saved, err := mockHostRepo.GetByID(host.ID)
		require.NoError(t, err)
		require.NotNil(t, saved)
		assert.Equal(t, host.ID, saved.ID)
	})

	// --- Ordering: name-uniqueness runs BEFORE the probe -------------
	//
	// A duplicate name should be rejected without paying the cost of an
	// SSH dial. This pins the documented order (validate -> name unique
	// -> PEM -> connection test) and guards a regression that probes
	// first, which would make a dup-name request needlessly slow and
	// would mask the HOST_ALREADY_EXISTS code behind a probe failure.
	t.Run("duplicate name is rejected without invoking the connection probe", func(t *testing.T) {
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("probe should not have been called"))
		mockEncryption := testutils.NewMockEncryptionService()

		// Seed an existing host with the same name for this owner.
		mockHostRepo.AddHost(&models.Host{
			ID:      "existing-host",
			Name:    "test-host",
			OwnerID: userID,
		})

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())
		host, err := service.Create(ctx, userID, newReq())

		require.Error(t, err)
		assert.Nil(t, host)
		appErr := requireAppError(t, err, "HOST_ALREADY_EXISTS", http.StatusBadRequest)
		assert.Equal(t, `Host with name "test-host" already exists`, appErr.Message,
			"the message must name the taken host name")
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
		mockHostRepo := testutils.NewMockHostRepository()
		mockAccessRepo := testutils.NewMockHostAccessRepository()
		mockConnManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("dial tcp: connection refused"))

		encryptCalled := false
		mockEncryption := testutils.NewMockEncryptionServiceWithCustomFunctions(
			func(string) ([]byte, string, error) {
				encryptCalled = true
				return []byte("x"), "fp", nil
			},
		)

		service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager, mockEncryption, newFakeSSHInvalidator())
		_, err := service.Create(ctx, userID, newReq())
		require.Error(t, err)

		assert.False(t, encryptCalled,
			"the SSH key must NOT be encrypted when the probe fails — the probe precedes encryption")
	})
}

// TestHostService_Create_ProbeFailureLogged covers the server-side log
// for a failed registration probe (issue 399): the underlying dial/ping
// error — discarded by the 400 mapping — must be logged with the
// requestID so an unreachable target is diagnosable from the server logs.
func TestHostService_Create_ProbeFailureLogged(t *testing.T) {
	capture := slogtest.CaptureDefault(t)

	probeErr := errors.New("ssh: handshake failed: ssh: unable to authenticate")
	mockHostRepo := testutils.NewMockHostRepository()
	mockAccessRepo := testutils.NewMockHostAccessRepository()
	mockConnManager := testutils.NewMockConnectionManagerWithTestError(probeErr)
	service := NewHostService(mockHostRepo, mockAccessRepo, mockConnManager,
		testutils.NewMockEncryptionService(), newFakeSSHInvalidator())

	ctx := contextutils.SetRequestID(context.Background(), "req-probe-log-1")
	_, err := service.Create(ctx, "test-user-123", &CreateHostRequest{
		Name:             "test-host",
		Hostname:         "example.com",
		Port:             22,
		SSHUsername:      "root",
		SSHPrivateKey:    testutils.GenerateValidPEMKey(),
		PodmanSocketPath: "/run/podman/podman.sock",
	})
	require.Error(t, err, "the probe failure must still answer a 400")

	logs := capture.Attrs(t, slog.LevelError)
	require.Len(t, logs, 1, "exactly one error log line for the failed probe")
	assert.Contains(t, logs[0]["error"], probeErr.Error(),
		"the underlying probe error must be logged")
	assert.Equal(t, "req-probe-log-1", logs[0]["requestID"],
		"the log line must carry the requestID")
	assert.Equal(t, "example.com", logs[0]["hostname"],
		"the log line must name the probed target")
}
