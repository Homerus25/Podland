package machines

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
// and the /{id} subtree is gated by RequireMachineAccess (read level
// suffices for the version endpoint, so no RequireMachinePermission layer).
func versionAccessRouter(machineRepo *testutils.MockMachineRepository,
	accessRepo *testutils.MockMachineAccessRepository, service *MachineService) http.Handler {
	r := http.NewServeMux()
	r.Handle("GET /api/machines/{id}/version",
		appmiddleware.RequireMachineAccess(machineRepo, accessRepo)(http.HandlerFunc(NewMachineHandler(service).Version)))
	return r
}

func newVersionHandlerTestService(t *testing.T, machineID, ownerID, callerID string, fetch versionFetcher) (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository) {
	t.Helper()
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineService(machineRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	service.fetchVersion = fetch
	if machineID != "" {
		machineRepo.AddMachine(&models.Machine{ID: machineID, Name: "m", OwnerID: ownerID})
		accessRepo.AddAccess(machineID, callerID, models.PermissionRead, time.Now())
	}
	return service, machineRepo, accessRepo
}

func TestMachineHandler_Version(t *testing.T) {
	t.Run("reachable machine returns 200 with both versions", func(t *testing.T) {
		service, machineRepo, accessRepo := newVersionHandlerTestService(t, "machine-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
			})
		router := versionAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/version", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotNil(t, body.PodmanVersion)
		require.NotNil(t, body.APIVersion)
		assert.Equal(t, "5.4.1", *body.PodmanVersion)
		assert.Equal(t, "5.4.1", *body.APIVersion)
	})

	t.Run("unreachable machine still returns 200 with null fields", func(t *testing.T) {
		service, machineRepo, accessRepo := newVersionHandlerTestService(t, "machine-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, context.DeadlineExceeded
			})
		router := versionAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/version", "owner-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Nil(t, body.PodmanVersion)
		assert.Nil(t, body.APIVersion)
		assert.NotContains(t, w.Body.String(), "deadline", "no failure detail may leak into the response")
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		service, machineRepo, accessRepo := newVersionHandlerTestService(t, "", "", "",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, nil
			})
		router := versionAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/no-such-machine/version", "owner-1"))

		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "MACHINE_NOT_FOUND")
	})

	t.Run("user without access is forbidden", func(t *testing.T) {
		service, machineRepo, accessRepo := newVersionHandlerTestService(t, "machine-1", "owner-1", "owner-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{}, nil
			})
		router := versionAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/version", "stranger-1"))

		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "ACCESS_DENIED")
	})

	t.Run("read-level grantee reaches the version endpoint", func(t *testing.T) {
		service, machineRepo, accessRepo := newVersionHandlerTestService(t, "machine-1", "owner-1", "reader-1",
			func(ctx context.Context) (versionReport, error) {
				return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
			})
		router := versionAccessRouter(machineRepo, accessRepo, service)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/version", "reader-1"))

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var body MachineVersionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotNil(t, body.PodmanVersion)
		assert.Equal(t, "5.4.1", *body.PodmanVersion)
	})
}

// The version endpoint's failure log carries the request ID (the status
// endpoint's rule); the log line is the one wiring fact worth pinning
// beyond the payload contract.
func TestMachineHandler_Version_LogCarriesRequestID(t *testing.T) {
	service, machineRepo, accessRepo := newVersionHandlerTestService(t, "machine-1", "owner-1", "owner-1",
		func(ctx context.Context) (versionReport, error) {
			return versionReport{}, context.DeadlineExceeded
		})
	router := versionAccessRouter(machineRepo, accessRepo, service)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	w := httptest.NewRecorder()
	req := requestAsUser(http.MethodGet, "/api/machines/machine-1/version", "owner-1")
	req = req.WithContext(contextutils.SetRequestID(req.Context(), "req-1"))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Contains(t, logs.String(), "machine version fetch failed")
	assert.Contains(t, logs.String(), "req-1")
}
