package hosts

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// versionAccessRouter mirrors the production wiring in internal/app/app.go:
// the JWT middleware's user-ID placement is stood in for by requestAsUser,
// and the /{id} subtree is gated by RequireHostAccess (read level
// suffices for the version endpoint, so no RequireHostPermission layer).
func versionAccessRouter(hostRepo *testutils.MockHostRepository,
	accessRepo *testutils.MockHostAccessRepository, service *HostService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/version",
		appmiddleware.RequireHostAccess(hostRepo, accessRepo)(http.HandlerFunc(NewHostHandler(service).Version)))
	return r
}

func newVersionHandlerTestService(t *testing.T, hostID, ownerID, callerID string, fetch versionFetcher) (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	t.Helper()
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	service := NewHostService(hostRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	service.fetchVersion = fetch
	if hostID != "" {
		hostRepo.AddHost(&models.Host{ID: hostID, Name: "m", OwnerID: ownerID})
		accessRepo.AddAccess(hostID, callerID, models.PermissionRead, time.Now())
	}
	return service, hostRepo, accessRepo
}

func TestHostHandler_Version(t *testing.T) {
	t.Run("reachable host returns 200 with both versions", func(t *testing.T) {
		service, hostRepo, accessRepo := newVersionHandlerTestService(t, "host-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
			})
		router := versionAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/version", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotNil(t, body.PodmanVersion)
		require.NotNil(t, body.APIVersion)
		assert.Equal(t, "5.4.1", *body.PodmanVersion)
		assert.Equal(t, "5.4.1", *body.APIVersion)
	})

	t.Run("unreachable host still returns 200 with null fields", func(t *testing.T) {
		service, hostRepo, accessRepo := newVersionHandlerTestService(t, "host-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, context.DeadlineExceeded
			})
		router := versionAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/version", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Nil(t, body.PodmanVersion)
		assert.Nil(t, body.APIVersion)
		assert.NotContains(t, w.Body.String(), "deadline", "no failure detail may leak into the response")
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		service, hostRepo, accessRepo := newVersionHandlerTestService(t, "", "", "",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, nil
			})
		router := versionAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/no-such-host/version", "owner-1"))

		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "HOST_NOT_FOUND")
	})

	t.Run("user without access is forbidden", func(t *testing.T) {
		service, hostRepo, accessRepo := newVersionHandlerTestService(t, "host-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, nil
			})
		router := versionAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/version", "stranger-1"))

		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "ACCESS_DENIED")
	})

	t.Run("read-level grantee reaches the version endpoint", func(t *testing.T) {
		service, hostRepo, accessRepo := newVersionHandlerTestService(t, "host-1", "owner-1", "reader-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
			})
		router := versionAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/version", "reader-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotNil(t, body.PodmanVersion)
		assert.Equal(t, "5.4.1", *body.PodmanVersion)
	})
}

// The version endpoint's failure log carries the request ID (the status
// endpoint's rule); the log line is the one wiring fact worth pinning
// beyond the payload contract.
func TestHostHandler_Version_LogCarriesRequestID(t *testing.T) {
	service, hostRepo, accessRepo := newVersionHandlerTestService(t, "host-1", "owner-1", "owner-1",
		func(ctx context.Context) (versionReport, error) {
			return versionReport{}, context.DeadlineExceeded
		})
	router := versionAccessRouter(hostRepo, accessRepo, service)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	w := httptest.NewRecorder()
	req := requestAsUser(http.MethodGet, "/api/hosts/host-1/version", "owner-1")
	req = req.WithContext(contextutils.SetRequestID(req.Context(), "req-1"))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, logs.String(), "host version fetch failed")
	assert.Contains(t, logs.String(), "req-1")
}
