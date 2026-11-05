package hosts

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCreateErrorsService builds a host service over mock repos for the
// create/update error-path handler tests. probeErr, when set, is what the
// connection manager's TestConnection answers.
func newCreateErrorsService(probeErr error) (*HostService, *testutils.MockHostRepository) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	var connManager *testutils.MockConnectionManager
	if probeErr != nil {
		connManager = testutils.NewMockConnectionManagerWithTestError(probeErr)
	} else {
		connManager = testutils.NewMockConnectionManager()
	}
	service := NewHostService(hostRepo, accessRepo, connManager,
		testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	return service, hostRepo
}

// validCreateBody is a well-formed registration body; the error subtests
// mutate the relevant field.
const validCreateBody = `{
	"name": "test-host",
	"hostname": "example.com",
	"port": 22,
	"sshUsername": "root",
	"sshPrivateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
	"podmanSocketPath": "/run/podman/podman.sock"
}`

// TestHostHandler_Create_EnrichedErrorMessages pins the registration
// 4xx contract over the HTTP surface (issue 399): the error codes stay
// stable while the envelope messages name the failed step and the reason
// — the frontend renders 4xx envelope messages verbatim, so the message
// IS the user-facing diagnosis.
func TestHostHandler_Create_EnrichedErrorMessages(t *testing.T) {
	t.Run("missing fields answer 400 VALIDATION_FAILED listing the field names", func(t *testing.T) {
		service, _ := newCreateErrorsService(nil)
		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body: strings.NewReader(`{
				"sshUsername": "root"
			}`),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "VALIDATION_FAILED", envelope.Error.Code)
		assert.Equal(t, "Missing required fields: name, hostname, sshPrivateKey",
			envelope.Error.Message,
			"the message must list the missing field names")
	})

	t.Run("non-PEM key answers 400 INVALID_SSH_KEY describing the PEM format", func(t *testing.T) {
		service, _ := newCreateErrorsService(nil)
		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body: strings.NewReader(`{
				"name": "test-host",
				"hostname": "example.com",
				"sshUsername": "root",
				"sshPrivateKey": "not-a-pem-key"
			}`),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "INVALID_SSH_KEY", envelope.Error.Code)
		assert.Contains(t, envelope.Error.Message, "-----BEGIN OPENSSH PRIVATE KEY-----",
			"the message must say what valid input looks like")
		assert.Contains(t, envelope.Error.Message, "-----END",
			"the message must name the marker pair")
	})

	t.Run("failed probe answers 400 CONNECTION_FAILED with the static message", func(t *testing.T) {
		service, _ := newCreateErrorsService(
			errors.New("ssh: handshake failed: ssh: unable to authenticate"))
		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body:   strings.NewReader(validCreateBody),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "CONNECTION_FAILED", envelope.Error.Code)
		assert.Equal(t, "Failed to connect to host during registration — check the hostname, port, SSH username, private key, and podman socket path",
			envelope.Error.Message,
			"the message must be the static text — the raw dial error is a port-scan oracle (pentest finding 1)")
		assert.NotContains(t, envelope.Error.Message, "handshake",
			"no SSH-level detail may reach the client")
	})

	t.Run("probe timeout answers 400 CONNECTION_TEST_TIMEOUT naming the bound", func(t *testing.T) {
		service, _ := newCreateErrorsService(&connections.TestTimeoutError{Bound: 10 * time.Second})
		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body:   strings.NewReader(validCreateBody),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "CONNECTION_TEST_TIMEOUT", envelope.Error.Code)
		assert.Equal(t, "Connection test timed out after 10s", envelope.Error.Message,
			"the message must state the timeout bound the probe exceeded")
	})

	t.Run("duplicate name answers 400 HOST_ALREADY_EXISTS naming the name", func(t *testing.T) {
		service, hostRepo := newCreateErrorsService(nil)
		hostRepo.AddHost(&models.Host{ID: "existing", Name: "test-host", OwnerID: "user-1"})
		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body:   strings.NewReader(validCreateBody),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "HOST_ALREADY_EXISTS", envelope.Error.Code)
		assert.Equal(t, `Host with name "test-host" already exists`, envelope.Error.Message,
			"the message must name the taken host name")
	})

	t.Run("blocked hostname answers 400 HOST_NOT_ALLOWED when the policy is enabled", func(t *testing.T) {
		// The opt-in host policy (pentest 2026-11-01 finding 1,
		// HOST_BLOCK_PRIVATE_RANGES): a blocked literal is
		// rejected before any dial. The connection manager is poisoned
		// with a probe error — a blocked host must never reach it.
		hostRepo := testutils.NewMockHostRepository()
		accessRepo := testutils.NewMockHostAccessRepository()
		connManager := testutils.NewMockConnectionManagerWithTestError(
			errors.New("probe must not run for a blocked host"))
		service := NewHostService(hostRepo, accessRepo, connManager,
			testutils.NewMockEncryptionService(), newFakeSSHInvalidator()).
			WithHostPolicy(NewHostPolicy(true))

		w := testutils.Perform(NewHostHandler(service).Create, testutils.Request{
			Method: http.MethodPost,
			Path:   "/api/hosts",
			UserID: "user-1",
			Body: strings.NewReader(`{
				"name": "test-host",
				"hostname": "10.0.0.5",
				"port": 22,
				"sshUsername": "root",
				"sshPrivateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
				"podmanSocketPath": "/run/podman/podman.sock"
			}`),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "HOST_NOT_ALLOWED", envelope.Error.Code)
		assert.Equal(t, "Host hostname is not allowed: this instance blocks private, loopback, link-local, CGNAT, and unspecified addresses",
			envelope.Error.Message)
		assert.Empty(t, connManager.TestConnectionCalls(),
			"a blocked host must never be dialed")
	})
}

// TestHostHandler_Update_EnrichedErrorMessages pins the update path's
// share of the enriched probe messages: Update crosses the same verifyKey
// probe, so a new key surfaces the same step-and-reason wording (issue
// 399's "inherits for free" ruling).
func TestHostHandler_Update_EnrichedErrorMessages(t *testing.T) {
	newUpdateReq := func() testutils.Request {
		return testutils.Request{
			Method:    http.MethodPut,
			Path:      "/api/hosts/host-1",
			UserID:    "owner-1",
			URLParams: map[string]string{"id": "host-1"},
			Body:      strings.NewReader(`{"sshPrivateKey": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----"}`),
		}
	}

	t.Run("non-PEM key answers 400 INVALID_SSH_KEY describing the PEM format", func(t *testing.T) {
		service, hostRepo := newCreateErrorsService(nil)
		hostRepo.AddHost(&models.Host{ID: "host-1", Name: "m", OwnerID: "owner-1"})

		w := testutils.Perform(NewHostHandler(service).Update, testutils.Request{
			Method:    http.MethodPut,
			Path:      "/api/hosts/host-1",
			UserID:    "owner-1",
			URLParams: map[string]string{"id": "host-1"},
			Body:      strings.NewReader(`{"sshPrivateKey": "not-a-pem-key"}`),
		})

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "INVALID_SSH_KEY", envelope.Error.Code)
		assert.Contains(t, envelope.Error.Message, "-----BEGIN OPENSSH PRIVATE KEY-----",
			"the update path shares the enriched PEM-format message (issue 399)")
	})

	t.Run("failed probe answers 400 CONNECTION_FAILED with the static message", func(t *testing.T) {
		service, hostRepo := newCreateErrorsService(errors.New("dial tcp: connection refused"))
		hostRepo.AddHost(&models.Host{ID: "host-1", Name: "m", OwnerID: "owner-1"})

		w := testutils.Perform(NewHostHandler(service).Update, newUpdateReq())

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "CONNECTION_FAILED", envelope.Error.Code)
		assert.Equal(t, "Failed to connect to host during registration — check the hostname, port, SSH username, private key, and podman socket path",
			envelope.Error.Message,
			"update shares the registration probe path, so the static message rides for free")
	})

	t.Run("probe timeout answers 400 CONNECTION_TEST_TIMEOUT naming the bound", func(t *testing.T) {
		service, hostRepo := newCreateErrorsService(&connections.TestTimeoutError{Bound: 10 * time.Second})
		hostRepo.AddHost(&models.Host{ID: "host-1", Name: "m", OwnerID: "owner-1"})

		w := testutils.Perform(NewHostHandler(service).Update, newUpdateReq())

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		envelope := testutils.DecodeErrorEnvelope(t, w.Result())
		assert.Equal(t, "CONNECTION_TEST_TIMEOUT", envelope.Error.Code)
		assert.Equal(t, "Connection test timed out after 10s", envelope.Error.Message)
	})
}
