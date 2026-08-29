package machines

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
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusAccessRouter mirrors the production wiring in internal/app/app.go:
// the JWT middleware's user-ID placement is stood in for by requestAsUser,
// and the /{id} subtree is gated by RequireMachineAccess (read level
// suffices for the status endpoint, so no RequireMachinePermission layer).
func statusAccessRouter(machineRepo *testutils.MockMachineRepository,
	accessRepo *testutils.MockMachineAccessRepository, service *MachineService) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			r.Get("/status", NewMachineHandler(service).Status)
		})
	})
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

func newStatusTestService(t *testing.T, machineID, ownerID, callerID string, probeErr error) (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository) {
	t.Helper()
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	var connManager *testutils.MockConnectionManager
	if probeErr != nil {
		connManager = testutils.NewMockConnectionManagerWithProbeError(probeErr)
	} else {
		connManager = testutils.NewMockConnectionManager()
	}
	service := NewMachineService(machineRepo, accessRepo, connManager, testutils.NewMockEncryptionService())
	if machineID != "" {
		machineRepo.AddMachine(&models.Machine{ID: machineID, Name: "m", OwnerID: ownerID})
		accessRepo.AddAccess(machineID, callerID, models.PermissionRead, time.Now())
	}
	return service, machineRepo, accessRepo
}

func TestMachineHandler_Status(t *testing.T) {
	t.Run("online machine returns 200 with status payload", func(t *testing.T) {
		service, machineRepo, accessRepo := newStatusTestService(t, "machine-1", "owner-1", "owner-1", nil)
		router := statusAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/status", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "online", body.Status)
		_, err := time.Parse(time.RFC3339, body.CheckedAt)
		require.NoError(t, err, "checkedAt must be RFC3339")
		assert.True(t, strings.HasSuffix(body.CheckedAt, "Z"), "checkedAt must be UTC: got %q", body.CheckedAt)
	})

	t.Run("offline machine still returns 200 with status payload", func(t *testing.T) {
		service, machineRepo, accessRepo := newStatusTestService(t, "machine-1", "owner-1", "owner-1",
			context.DeadlineExceeded)
		router := statusAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/status", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "offline", body.Status)
		assert.NotContains(t, w.Body.String(), "deadline", "no probe error detail may leak into the response")
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		service, machineRepo, accessRepo := newStatusTestService(t, "", "", "", nil)
		router := statusAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/no-such-machine/status", "owner-1"))

		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "MACHINE_NOT_FOUND")
	})

	t.Run("user without access is forbidden", func(t *testing.T) {
		service, machineRepo, accessRepo := newStatusTestService(t, "machine-1", "owner-1", "owner-1", nil)
		router := statusAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/status", "stranger-1"))

		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "ACCESS_DENIED")
	})

	t.Run("read-level grantee reaches the status endpoint", func(t *testing.T) {
		service, machineRepo, accessRepo := newStatusTestService(t, "machine-1", "owner-1", "reader-1", nil)
		router := statusAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/status", "reader-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineStatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "online", body.Status)
	})
}

