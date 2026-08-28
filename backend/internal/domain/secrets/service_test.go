package secrets

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
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

	service := NewSecretService(mockConnManager, testutils.NewMockMachineAccessRepository())

	assert.NotNil(t, service)
	assert.NotNil(t, service.connManager)
}

func TestSecretService_List_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewSecretService(mockConnManager, testutils.NewMockMachineAccessRepository())

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
	mu     sync.Mutex
	method string
	path   string
	query  url.Values
	// inspectCalls counts requests served on the single-secret inspect
	// endpoint (/libpod/secrets/{id}/json), so a test can pin that a
	// denied reveal never reached podman at all.
	inspectCalls int
}

// startRecordingPodman starts an HTTP test server performing the
// fake-podman bookkeeping every endpoint-specific fake shares: recording
// each request (method, path, raw query) on the returned fake and serving
// the /_ping connection handshake. Everything else is delegated to serve,
// which composes the endpoints under test. The URL scheme is tcp because
// bindings.NewConnection only understands tcp/unix/ssh and the httptest
// listener is a plain TCP listener.
func startRecordingPodman(t *testing.T, serve func(w http.ResponseWriter, r *http.Request, fake *fakePodman)) (*fakePodman, *httptest.Server) {
	t.Helper()
	fake := &fakePodman{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.method = r.Method
		fake.path = r.URL.Path
		fake.query = r.URL.Query()
		fake.mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		}
		serve(w, r, fake)
	}))
	t.Cleanup(server.Close)
	return fake, server
}

// newFakePodmanServer starts a fake podman speaking enough of the service
// API for secrets.List: /_ping (the connection handshake, via
// startRecordingPodman) and /libpod/secrets/json, answering with the given
// canned response body.
func newFakePodmanServer(t *testing.T, secretsJSON string) (*fakePodman, *httptest.Server) {
	return startRecordingPodman(t, func(w http.ResponseWriter, r *http.Request, fake *fakePodman) {
		if strings.HasSuffix(r.URL.Path, "/libpod/secrets/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(secretsJSON))
			return
		}
		t.Errorf("fake podman received unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
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
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewSecretService(connManager, accessRepo)

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
	service := NewSecretService(connManager, testutils.NewMockMachineAccessRepository())

	result, err := service.List(context.Background(), "test-user", "test-machine")

	require.NoError(t, err)
	assert.Equal(t, []SecretListResponse{}, result, "no secrets must map to an empty JSON array, not null")
	assert.True(t, strings.HasSuffix(fake.path, "/libpod/secrets/json"),
		"the fake podman must have served the secrets list endpoint, got %q", fake.path)
}

// --- Service seam: Inspect (metadata + admin-only reveal) ---

// newSecretInspectServer starts a fake podman speaking enough of the
// service API for secrets.Inspect: /_ping (the connection handshake, via
// startRecordingPodman) and the single-secret endpoint
// /libpod/secrets/{id}/json, answering with the configured status and
// canned body — podman's error wire shape (errorhandling.ErrorModel JSON)
// when status >= 400.
func newSecretInspectServer(t *testing.T, status int, body string) (*fakePodman, *httptest.Server) {
	return startRecordingPodman(t, func(w http.ResponseWriter, r *http.Request, fake *fakePodman) {
		if strings.Contains(r.URL.Path, "/libpod/secrets/") && strings.HasSuffix(r.URL.Path, "/json") {
			fake.mu.Lock()
			fake.inspectCalls++
			fake.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		t.Errorf("fake podman received unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
}

// TestSecretService_Get_MetadataOnly pins the read-level contract: a
// metadata inspect must hit podman's single-secret endpoint WITHOUT the
// showsecret param, and the value that podman's wire response may carry
// must not reach the response DTO.
func TestSecretService_Get_MetadataOnly(t *testing.T) {
	inspectJSON := `{
		"ID": "3f2e1d0c",
		"CreatedAt": "2026-01-02T15:04:05Z",
		"UpdatedAt": "2026-01-03T16:05:06Z",
		"Spec": {
			"Name": "db-password",
			"Driver": {"Name": "file", "Options": {}},
			"Labels": {"app": "web"}
		},
		"SecretData": "must-not-leak"
	}`
	fake, server := newSecretInspectServer(t, http.StatusOK, inspectJSON)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("test-machine", "reader-1", models.PermissionRead, time.Now())
	service := NewSecretService(connManager, accessRepo)

	result, err := service.Get(context.Background(), "reader-1", "test-machine", "db-password", false)

	require.NoError(t, err, "metadata inspect for a read-level grantee must succeed")

	fake.mu.Lock()
	gotMethod, gotPath, gotQuery, gotInspectCalls := fake.method, fake.path, fake.query, fake.inspectCalls
	fake.mu.Unlock()
	assert.Equal(t, http.MethodGet, gotMethod, "bindings must issue a GET")
	assert.True(t, strings.HasSuffix(gotPath, "/libpod/secrets/db-password/json"),
		"bindings must hit the podman secret inspect endpoint, got %q", gotPath)
	assert.Equal(t, 1, gotInspectCalls, "exactly one inspect call must reach podman")
	assert.NotContains(t, gotQuery, "showsecret",
		"a metadata inspect must not ask podman for the value, got query %v", gotQuery)

	require.NotNil(t, result, "a successful inspect must return a response")
	assert.Equal(t, &SecretInspectResponse{
		SecretListResponse: SecretListResponse{
			ID:        "3f2e1d0c",
			Name:      "db-password",
			Driver:    "file",
			CreatedAt: "2026-01-02T15:04:05Z",
			UpdatedAt: "2026-01-03T16:05:06Z",
			Labels:    map[string]string{"app": "web"},
		},
	}, result, "the response must be the metadata DTO with NO value")
	assert.Empty(t, result.SecretData, "a metadata inspect must never carry the secret value")

	assert.Equal(t, []string{"test-machine"}, connManager.released,
		"the connection lease must be released after the call")
}

// TestSecretService_Get_AdminRevealReturnsValue pins the admin reveal
// contract: an admin caller's ?reveal=true must ask podman for the value
// (showsecret=true) and surface it in the response.
func TestSecretService_Get_AdminRevealReturnsValue(t *testing.T) {
	inspectJSON := `{
		"ID": "3f2e1d0c",
		"CreatedAt": "2026-01-02T15:04:05Z",
		"UpdatedAt": "2026-01-03T16:05:06Z",
		"Spec": {
			"Name": "db-password",
			"Driver": {"Name": "file", "Options": {}}
		},
		"SecretData": "hunter2"
	}`
	fake, server := newSecretInspectServer(t, http.StatusOK, inspectJSON)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("test-machine", "admin-1", models.PermissionAdmin, time.Now())
	service := NewSecretService(connManager, accessRepo)

	result, err := service.Get(context.Background(), "admin-1", "test-machine", "db-password", true)

	require.NoError(t, err, "an admin reveal must succeed")

	fake.mu.Lock()
	gotQuery, gotInspectCalls := fake.query, fake.inspectCalls
	fake.mu.Unlock()
	assert.Equal(t, 1, gotInspectCalls, "exactly one inspect call must reach podman")
	require.NotNil(t, gotQuery, "the inspect request must carry a query")
	assert.Equal(t, "true", gotQuery.Get("showsecret"),
		"an admin reveal must set WithShowSecret so podman fills SecretData")

	require.NotNil(t, result, "a successful reveal must return a response")
	assert.Equal(t, "hunter2", result.SecretData, "the reveal must surface the secret value")
	assert.Equal(t, "db-password", result.Name, "the reveal still carries the metadata")
}

// TestSecretService_Get_RevealBelowAdminIsDenied pins the level ruling:
// ?reveal=true from a read- or write-level caller is refused with
// ACCESS_DENIED — NOT silently degraded to metadata, so the UI can
// distinguish "no permission" from "no value" — and podman is never asked
// for the secret (the deny fires before a connection lease is taken).
func TestSecretService_Get_RevealBelowAdminIsDenied(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{name: "read-level caller", level: models.PermissionRead},
		{name: "write-level caller", level: models.PermissionWrite},
		{name: "caller without an access row", level: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, server := newSecretInspectServer(t, http.StatusOK, `{}`)
			connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
			accessRepo := testutils.NewMockMachineAccessRepository()
			if tt.level != "" {
				accessRepo.AddAccess("test-machine", "caller-1", tt.level, time.Now())
			}
			service := NewSecretService(connManager, accessRepo)

			result, err := service.Get(context.Background(), "caller-1", "test-machine", "db-password", true)

			assert.Equal(t, appErrors.ErrAccessDenied, err,
				"a below-admin reveal must be refused with ACCESS_DENIED, got %v", err)
			assert.Nil(t, result, "a denied reveal must not return a response")

			fake.mu.Lock()
			gotInspectCalls := fake.inspectCalls
			fake.mu.Unlock()
			assert.Equal(t, 0, gotInspectCalls, "a denied reveal must never reach podman")
			assert.Empty(t, connManager.released,
				"a denied reveal must not take a connection lease at all")
		})
	}
}

// TestSecretService_Get_RevealPermissionLookupFailure pins that a failing
// permission lookup surfaces as a 500 INTERNAL_ERROR, never a silent
// allow/deny guess (the middleware's contract for the same situation).
func TestSecretService_Get_RevealPermissionLookupFailure(t *testing.T) {
	_, server := newSecretInspectServer(t, http.StatusOK, `{}`)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	accessRepo := testutils.NewMockMachineAccessRepository().WithGetPermissionError(errors.New("db down"))
	service := NewSecretService(connManager, accessRepo)

	result, err := service.Get(context.Background(), "caller-1", "test-machine", "db-password", true)

	assert.Equal(t, appErrors.ErrInternalError, err, "a failing lookup must surface as INTERNAL_ERROR")
	assert.Nil(t, result)
	assert.Empty(t, connManager.released, "no connection lease may be taken when the lookup fails")
}

// TestSecretService_Get_UnknownSecretIsNotFound pins the 404 mapping: a
// podman not-found answer (errorhandling.ErrorModel with response 404,
// the bindings' decoding of the service API's error body) surfaces as the
// domain's SECRET_NOT_FOUND sentinel for both metadata and reveal calls.
func TestSecretService_Get_UnknownSecretIsNotFound(t *testing.T) {
	notFoundBody := `{"cause":"no such secret","message":"no such secret with name or id \"nope\"","response":404}`

	t.Run("metadata inspect", func(t *testing.T) {
		_, server := newSecretInspectServer(t, http.StatusNotFound, notFoundBody)
		connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
		service := NewSecretService(connManager, testutils.NewMockMachineAccessRepository())

		result, err := service.Get(context.Background(), "reader-1", "test-machine", "nope", false)

		assert.Equal(t, appErrors.ErrSecretNotFound, err, "a podman 404 must map to SECRET_NOT_FOUND")
		assert.Nil(t, result)
		assert.Equal(t, []string{"test-machine"}, connManager.released,
			"the lease must still be released on a podman 404")
	})

	t.Run("admin reveal", func(t *testing.T) {
		_, server := newSecretInspectServer(t, http.StatusNotFound, notFoundBody)
		connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
		accessRepo := testutils.NewMockMachineAccessRepository()
		accessRepo.AddAccess("test-machine", "admin-1", models.PermissionAdmin, time.Now())
		service := NewSecretService(connManager, accessRepo)

		result, err := service.Get(context.Background(), "admin-1", "test-machine", "nope", true)

		assert.Equal(t, appErrors.ErrSecretNotFound, err, "a podman 404 on reveal must map to SECRET_NOT_FOUND")
		assert.Nil(t, result)
	})
}

// TestSecretService_Get_OtherPodmanErrorsStayGeneric pins that only a 404
// ErrorModel is special-cased: any other podman failure (here a 500 error
// model) surfaces as a non-AppError the handler maps to 500, never as
// SECRET_NOT_FOUND.
func TestSecretService_Get_OtherPodmanErrorsStayGeneric(t *testing.T) {
	_, server := newSecretInspectServer(t, http.StatusInternalServerError,
		`{"cause":"boom","message":"internal podman error","response":500}`)
	connManager := &stubConnManager{connCtx: tunnelCtxTo(t, server)}
	service := NewSecretService(connManager, testutils.NewMockMachineAccessRepository())

	result, err := service.Get(context.Background(), "reader-1", "test-machine", "db-password", false)

	assert.Error(t, err, "a podman 500 must surface as an error")
	assert.NotEqual(t, appErrors.ErrSecretNotFound, err, "a podman 500 must not masquerade as 404")
	var appErr *appErrors.AppError
	assert.NotErrorAs(t, err, &appErr, "the raw error stays non-AppError so the handler 500s it")
	assert.Nil(t, result)
}

// TestSecretService_Get_ConnectionFailure pins the connection seam for
// inspect: a failed tunnel maps to ErrConnectionFailed (502), mirroring
// List and the other machine-rider services.
func TestSecretService_Get_ConnectionFailure(t *testing.T) {
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewSecretService(mockConnManager, testutils.NewMockMachineAccessRepository())

	result, err := service.Get(context.Background(), "reader-1", "test-machine", "db-password", false)

	assert.Equal(t, appErrors.ErrConnectionFailed, err, "a failed connection must map to ErrConnectionFailed")
	assert.Nil(t, result)
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
