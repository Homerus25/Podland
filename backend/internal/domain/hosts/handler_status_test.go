package hosts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusAccessRouter mirrors the production wiring in internal/app/app.go:
// the JWT middleware's user-ID placement is stood in for by requestAsUser,
// and the /{id} subtree is gated by RequireHostAccess (read level
// suffices for the status endpoint, so no RequireHostPermission layer).
func statusAccessRouter(hostRepo *testutils.MockHostRepository,
	accessRepo *testutils.MockHostAccessRepository, service *HostService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/status",
		appmiddleware.RequireHostAccess(hostRepo, accessRepo)(http.HandlerFunc(NewHostHandler(service).Status)))
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

func newStatusTestService(t *testing.T, hostID, ownerID, callerID string, probeErr error) (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	t.Helper()
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	var connManager *testutils.MockConnectionManager
	if probeErr != nil {
		connManager = testutils.NewMockConnectionManagerWithProbeError(probeErr)
	} else {
		connManager = testutils.NewMockConnectionManager()
	}
	service := NewHostService(hostRepo, accessRepo, connManager, testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	if hostID != "" {
		hostRepo.AddHost(&models.Host{ID: hostID, Name: "m", OwnerID: ownerID})
		accessRepo.AddAccess(hostID, callerID, models.PermissionRead, time.Now())
	}
	return service, hostRepo, accessRepo
}

func TestHostHandler_Status(t *testing.T) {
	t.Run("online host returns 200 with status payload", func(t *testing.T) {
		service, hostRepo, accessRepo := newStatusTestService(t, "host-1", "owner-1", "owner-1", nil)
		router := statusAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/status", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "online", body.Status)
		_, err := time.Parse(time.RFC3339, body.CheckedAt)
		require.NoError(t, err, "checkedAt must be RFC3339")
		assert.True(t, strings.HasSuffix(body.CheckedAt, "Z"), "checkedAt must be UTC: got %q", body.CheckedAt)
	})

	t.Run("offline host still returns 200 with status payload", func(t *testing.T) {
		service, hostRepo, accessRepo := newStatusTestService(t, "host-1", "owner-1", "owner-1",
			context.DeadlineExceeded)
		router := statusAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/status", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "offline", body.Status)
		assert.NotContains(t, w.Body.String(), "deadline", "no probe error detail may leak into the response")
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		service, hostRepo, accessRepo := newStatusTestService(t, "", "", "", nil)
		router := statusAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/no-such-host/status", "owner-1"))

		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "HOST_NOT_FOUND")
	})

	t.Run("user without access is forbidden", func(t *testing.T) {
		service, hostRepo, accessRepo := newStatusTestService(t, "host-1", "owner-1", "owner-1", nil)
		router := statusAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/status", "stranger-1"))

		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "ACCESS_DENIED")
	})

	t.Run("read-level grantee reaches the status endpoint", func(t *testing.T) {
		service, hostRepo, accessRepo := newStatusTestService(t, "host-1", "owner-1", "reader-1", nil)
		router := statusAccessRouter(hostRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/status", "reader-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body HostStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "online", body.Status)
	})
}
