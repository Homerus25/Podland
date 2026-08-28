package secrets

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/bindings"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestNewSecretService_WithMockConnManager verifies that SecretService can be
// constructed with a mock ConnectionManagerInterface, enabling unit testing.
func TestNewSecretService_WithMockConnManager(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()

	service := NewSecretService(mockConnManager)

	assert.NotNil(t, service)
	assert.NotNil(t, service.connManager)
}

func TestSecretService_List_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewSecretService(mockConnManager)

	result, err := service.List(ctx, "test-user", "test-machine")

	assert.Error(t, err, "Should return error when connection fails")
	assert.Equal(t, err, appErrors.ErrConnectionFailed, "Should map to ErrConnectionFailed")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

// --- Service seam: List through a fake podman service API ---
//
// The bindings boundary (go.podman.io/podman/v6/pkg/bindings/secrets) is a
// package-level function that talks HTTP to whatever *bindings.Connection
// the tunnel context carries, so the deepest seam a unit test can reach is
// a fake podman HTTP server: GetConnection hands back a context built by
// bindings.NewConnection against the fake, and the fake answers the real
// endpoints (/_ping and /libpod/secrets/json) with podman's real wire
// shape. That lets the tests below assert both the bindings invocation and
// the entity→DTO mapping without a real machine.

// fakePodman is a stand-in podman service API. It records every request it
// serves so tests can assert what the bindings layer actually sent.
type fakePodman struct {
	mu       sync.Mutex
	method   string
	path     string
	bodyJSON string
}

// newFakePodmanServer starts an HTTP server speaking enough of the podman
// service API for secrets.List: /_ping (the connection handshake) and
// /libpod/secrets/json, answering with the given canned response body.
// The URL scheme is tcp because bindings.NewConnection only understands
// tcp/unix/ssh and the httptest listener is a plain TCP listener.
func newFakePodmanServer(t *testing.T, secretsJSON string) (*fakePodman, *httptest.Server) {
	t.Helper()
	fake := &fakePodman{bodyJSON: secretsJSON}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.method = r.Method
		fake.path = r.URL.Path
		fake.mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/libpod/secrets/json"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(secretsJSON))
		default:
			t.Errorf("fake podman received unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return fake, server
}

// stubConnManager is a machines.ConnectionManagerInterface whose
// GetConnection always returns the pre-built tunnel context (and records
// the Release call so tests can assert the lease is handed back).
type stubConnManager struct {
	connCtx  context.Context
	released []string
}

func (m *stubConnManager) GetConnection(machineID, userID string) (context.Context, error) {
	return m.connCtx, nil
}

func (m *stubConnManager) Release(machineID string) {
	m.released = append(m.released, machineID)
}

func (m *stubConnManager) TestConnection(ctx context.Context, hostname string, port int, sshUsername, sshPrivateKey, podmanSocketPath string) error {
	return nil
}

// tunnelCtxTo builds a context carrying a bindings connection to the given
// HTTP server, the way the real connection factory does for SSH tunnels.
func tunnelCtxTo(t *testing.T, server *httptest.Server) context.Context {
	t.Helper()
	uri := "tcp://" + server.Listener.Addr().String()
	connCtx, err := bindings.NewConnection(context.Background(), uri)
	require.NoError(t, err, "fake podman must accept the connection handshake")
	return connCtx
}

func TestSecretService_List_CallsBindingsAndMaps(t *testing.T) {
	// Podman's wire shape for GET /libpod/secrets/json: SecretInfoReport
	// objects (field-name keys, Spec nested). SecretData appears here even
	// though a real list never populates it, to pin that mapping drops it.
	secretsJSON := `[
		{
			"ID": "3f2e1d0c",
			"CreatedAt": "2026-01-02T15:04:05Z",
			"UpdatedAt": "2026-01-03T16:05:06Z",
			"Spec": {
				"Name": "db-password",
				"Driver": {"Name": "file", "Options": {}},
				"Labels": {"app": "web"}
			},
			"SecretData": "must-not-leak"
		},
		{
			"ID": "9a8b7c6d",
			"CreatedAt": "2026-02-04T10:11:12Z",
			"UpdatedAt": "2026-02-04T10:11:12Z",
			"Spec": {
				"Name": "api-token",
				"Driver": {"Name": "file", "Options": null}
			}
		}
	]`
	fake, server := newFakePodmanServer(t, secretsJSON)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	service := NewSecretService(connManager)

	result, err := service.List(context.Background(), "test-user", "test-machine")

	require.NoError(t, err, "List against the fake podman must succeed")

	fake.mu.Lock()
	gotMethod, gotPath := fake.method, fake.path
	fake.mu.Unlock()
	assert.Equal(t, http.MethodGet, gotMethod, "bindings must issue a GET")
	assert.True(t, strings.HasSuffix(gotPath, "/libpod/secrets/json"),
		"bindings must hit the podman secrets list endpoint, got %q", gotPath)

	expected := []SecretListResponse{
		{
			ID:        "3f2e1d0c",
			Name:      "db-password",
			Driver:    "file",
			CreatedAt: "2026-01-02T15:04:05Z",
			UpdatedAt: "2026-01-03T16:05:06Z",
			Labels:    map[string]string{"app": "web"},
		},
		{
			ID:        "9a8b7c6d",
			Name:      "api-token",
			Driver:    "file",
			CreatedAt: "2026-02-04T10:11:12Z",
			UpdatedAt: "2026-02-04T10:11:12Z",
		},
	}
	assert.Equal(t, expected, result, "entities must map to metadata-only DTOs")

	assert.Equal(t, []string{"test-machine"}, connManager.released,
		"the connection lease must be released after the call")
}

func TestSecretService_List_EmptyMachineReturnsEmptyList(t *testing.T) {
	fake, server := newFakePodmanServer(t, `[]`)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	service := NewSecretService(connManager)

	result, err := service.List(context.Background(), "test-user", "test-machine")

	require.NoError(t, err)
	assert.Equal(t, []SecretListResponse{}, result, "no secrets must map to an empty JSON array, not null")
	assert.True(t, strings.HasSuffix(fake.path, "/libpod/secrets/json"),
		"the fake podman must have served the secrets list endpoint, got %q", fake.path)
}

// --- Mapping (pure functions) ---

// TestFromSecretInfoReport_DropsSecretData pins the redaction discipline:
// even when a report carries SecretData (only the admin reveal path may),
// the mapped DTO has no field that could hold it, so the value cannot leak
// into a list response.
func TestFromSecretInfoReport_DropsSecretData(t *testing.T) {
	report := &entities.SecretInfoReport{
		ID:         "3f2e1d0c",
		CreatedAt:  time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 1, 3, 16, 5, 6, 0, time.UTC),
		Spec:       entities.SecretSpec{Name: "db-password", Driver: entities.SecretDriverSpec{Name: "file"}},
		SecretData: "must-not-leak",
	}

	dto := FromSecretInfoReport(report)

	assert.Equal(t, "3f2e1d0c", dto.ID)
	assert.Equal(t, "db-password", dto.Name)
	assert.Equal(t, "file", dto.Driver)
	assert.Equal(t, "2026-01-02T15:04:05Z", dto.CreatedAt)
	assert.Equal(t, "2026-01-03T16:05:06Z", dto.UpdatedAt)
}

// TestFromSecretInfoReport_ZeroTimesAndNilReport pins the degenerate
// mapping cases: a zero timestamp maps to "" and a nil report to the zero
// DTO rather than a panic.
func TestFromSecretInfoReport_ZeroTimesAndNilReport(t *testing.T) {
	assert.Equal(t, SecretListResponse{}, FromSecretInfoReport(nil),
		"nil report must map to the zero DTO")

	dto := FromSecretInfoReport(&entities.SecretInfoReport{
		Spec: entities.SecretSpec{Name: "empty"},
	})
	assert.Empty(t, dto.CreatedAt, "zero CreatedAt must map to \"\"")
	assert.Empty(t, dto.UpdatedAt, "zero UpdatedAt must map to \"\"")
	assert.Nil(t, dto.Labels, "absent labels must stay nil so json omits them")
}

// TestToSecretListResponse_NilSliceKeepsEmptyArray pins that a nil report
// slice still maps to an empty (non-nil) slice, so the HTTP response is
// `[]` rather than `null`.
func TestToSecretListResponse_NilSliceKeepsEmptyArray(t *testing.T) {
	assert.Equal(t, []SecretListResponse{}, ToSecretListResponse(nil))
}
