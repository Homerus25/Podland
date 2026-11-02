package containers

import (
	"encoding/json"
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
	"go.podman.io/podman/v6/libpod/define"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// allStatsRequest serves GET /api/machines/{id}/containers/stats (the
// all-stats literal route) through the handler with an authenticated test
// identity.
func allStatsRequest(stats http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(stats, testutils.Request{
		Method: http.MethodGet,
		Path:   "/api/machines/test-machine/containers/stats",
		UserID: "test-user",
		URLParams: map[string]string{
			"id": "test-machine",
		},
	})
}

// containerStatsRequest serves GET
// /api/machines/{id}/containers/{containerId}/stats through the handler
// with an authenticated test identity.
func containerStatsRequest(stats http.HandlerFunc, containerID string) *httptest.ResponseRecorder {
	return testutils.Perform(stats, testutils.Request{
		Method: http.MethodGet,
		Path:   "/api/machines/test-machine/containers/" + containerID + "/stats",
		UserID: "test-user",
		URLParams: map[string]string{
			"id":          "test-machine",
			"containerId": containerID,
		},
	})
}

// runningContainerStats is one running container's stats entity, the
// shape podman's one-shot snapshot answers for it.
func runningContainerStats() define.ContainerStats {
	return define.ContainerStats{
		ContainerID: "container-123",
		Name:        "web",
		CPU:         12.5,
		MemUsage:    1048576,
		MemLimit:    2147483648,
		MemPerc:     0.05,
		Network: map[string]define.ContainerNetworkStats{
			"eth0": {RxBytes: 1000, TxBytes: 500},
		},
		BlockInput:  4096,
		BlockOutput: 8192,
		PIDs:        7,
	}
}

// TestContainerHandler_Stats_AllSnapshot pins the all-stats contract: 200
// with the list envelope, one DTO per running container, raw values under
// their camelCase wire tags.
func TestContainerHandler_Stats_AllSnapshot(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func(ids []string, all bool) ([]define.ContainerStats, error) {
			return []define.ContainerStats{runningContainerStats()}, nil
		},
	}

	w := allStatsRequest(newTestHandler(engine).Stats)

	require.Equal(t, http.StatusOK, w.Code, "the all-stats route must answer 200")
	var body ContainerStatsResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the all-stats route must answer the list envelope")
	require.Len(t, body.Containers, 1)
	assert.Equal(t, "container-123", body.Containers[0].ID)
	assert.Equal(t, 1, body.Total)
	assert.NotEmpty(t, body.Timestamp)

	// Pin the wire tags: the raw numeric fields ride camelCase keys.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, float64(1), raw["total"])
	containers, ok := raw["containers"].([]any)
	require.True(t, ok, "containers must be a JSON array")
	entry, ok := containers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "container-123", entry["id"])
	assert.Equal(t, "web", entry["name"])
	assert.Equal(t, float64(12.5), entry["cpuPercent"])
	assert.Equal(t, float64(1048576), entry["memoryUsage"])
	assert.Equal(t, float64(2147483648), entry["memoryLimit"])
	assert.Equal(t, float64(0.05), entry["memoryPercent"])
	assert.Equal(t, float64(1000), entry["networkBytesIn"])
	assert.Equal(t, float64(500), entry["networkBytesOut"])
	assert.Equal(t, float64(4096), entry["blockBytesIn"])
	assert.Equal(t, float64(8192), entry["blockBytesOut"])
	assert.Equal(t, float64(7), entry["pids"])
}

// TestContainerHandler_Stats_EmptySnapshotIsLegal200 pins the legal empty
// answer: nothing running answers 200 with an empty array, never null.
func TestContainerHandler_Stats_EmptySnapshotIsLegal200(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return nil, nil
		},
	}

	w := allStatsRequest(newTestHandler(engine).Stats)

	require.Equal(t, http.StatusOK, w.Code)
	var body ContainerStatsResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body.Containers, "an empty snapshot must answer an empty array, not null")
	assert.Empty(t, body.Containers)
}

// TestContainerHandler_ContainerStats_SingleDTO pins the per-container
// contract: 200 with the single container's stats DTO.
func TestContainerHandler_ContainerStats_SingleDTO(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func(ids []string, all bool) ([]define.ContainerStats, error) {
			assert.Equal(t, []string{"container-123"}, ids, "the per-container route must target the container ID")
			assert.False(t, all, "the per-container route must not set the all flag")
			return []define.ContainerStats{runningContainerStats()}, nil
		},
	}

	w := containerStatsRequest(newTestHandler(engine).ContainerStats, "container-123")

	require.Equal(t, http.StatusOK, w.Code, "the per-container stats route must answer 200")
	var body ContainerStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the per-container stats route must answer the stats DTO")
	assert.Equal(t, "container-123", body.ID)
	assert.Equal(t, "web", body.Name)
	assert.Equal(t, 12.5, body.CPUPercent)
	assert.Equal(t, uint64(1048576), body.MemoryUsage)
	assert.Equal(t, uint64(2147483648), body.MemoryLimit)
	assert.Equal(t, 0.05, body.MemoryPercent)
	assert.Equal(t, uint64(1000), body.NetworkBytesIn)
	assert.Equal(t, uint64(500), body.NetworkBytesOut)
	assert.Equal(t, uint64(4096), body.BlockBytesIn)
	assert.Equal(t, uint64(8192), body.BlockBytesOut)
	assert.Equal(t, uint64(7), body.PIDs)
}

// TestContainerHandler_ContainerStats_MissingContainerIDAnswers400 pins
// the URL-param guard: a request without a containerId answers 400
// MISSING_CONTAINER_ID through the envelope.
func TestContainerHandler_ContainerStats_MissingContainerIDAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{}
	// statsFn stays unarmed: the guard must answer before any engine
	// call, and an unexpected call panics.

	w := testutils.Perform(newTestHandler(engine).ContainerStats, testutils.Request{
		Method: http.MethodGet,
		Path:   "/api/machines/test-machine/containers//stats",
		UserID: "test-user",
		URLParams: map[string]string{
			"id": "test-machine",
		},
	})

	require.Equal(t, http.StatusBadRequest, w.Code, "a missing containerId must answer 400")
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "MISSING_CONTAINER_ID", body.Error.Code)
}

// TestContainerHandler_ContainerStats_MissingContainerAnswers404 pins the
// documented 404 envelope end to end: the real service classifies
// podman's not-found answer and the web envelope maps the sentinel, with
// podman's own message riding along as the disambiguator.
func TestContainerHandler_ContainerStats_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}

	w := containerStatsRequest(newTestHandler(engine).ContainerStats, "container-123")

	require.Equal(t, http.StatusNotFound, w.Code, "a container without stats must answer 404")
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "CONTAINER_NOT_FOUND", body.Error.Code)
	assert.Contains(t, body.Error.Message, "no such container", "podman's own message must ride along")
}

// TestContainerHandler_ContainerStats_EmptySnapshotAnswers404 pins the
// defensive terminal: a clean empty answer for a targeted container —
// the shape podman's own stats handler refuses with a 404 — gets the
// same not-found sentinel rather than a fabricated zero DTO.
func TestContainerHandler_ContainerStats_EmptySnapshotAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return nil, nil
		},
	}

	w := containerStatsRequest(newTestHandler(engine).ContainerStats, "container-123")

	require.Equal(t, http.StatusNotFound, w.Code)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "CONTAINER_NOT_FOUND", body.Error.Code)
}

// --- Route wiring: the containers subtree serves stats at read level ---
//
// These tests mount the containers routes on a ServeMux exactly the way
// internal/app/app.go does (RequireMachineAccess wrapping the routes,
// the literal GET /stats winning over the /{containerId} wildcard by
// ServeMux precedence, the per-container GET /stats beside /logs and
// /inspect) so the read-level gate and the literal-route win the ACs
// promise are exercised through the real middleware.

func statsAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, handler *ContainerHandler) http.Handler {
	machineAccess := appmiddleware.RequireMachineAccess(machineRepo, accessRepo)
	r := http.NewServeMux()
	// The literal /stats route must beat the /{containerId} wildcard or
	// an all-stats call gets swallowed as a container named "stats".
	r.Handle("GET /api/machines/{id}/containers/stats", machineAccess(http.HandlerFunc(handler.Stats)))
	r.Handle("GET /api/machines/{id}/containers/{containerId}", machineAccess(http.HandlerFunc(handler.Get)))
	r.Handle("GET /api/machines/{id}/containers/{containerId}/stats", machineAccess(http.HandlerFunc(handler.ContainerStats)))
	return r
}

func statsRequestAsUser(router http.Handler, method, target, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(contextutils.SetUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestContainerHandler_Stats_ReadLevelGranteeReachesBothRoutes pins the
// read-level gate through the real middleware: a read-level grantee
// reaches both stats routes, like list/get/logs — no write gate.
func TestContainerHandler_Stats_ReadLevelGranteeReachesBothRoutes(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return []define.ContainerStats{runningContainerStats()}, nil
		},
	}
	router := statsAccessRouter(machineRepo, accessRepo, newTestHandler(engine))

	w := statsRequestAsUser(router, http.MethodGet, "/api/machines/machine-1/containers/stats", "reader-1")
	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the all-stats route")

	w = statsRequestAsUser(router, http.MethodGet, "/api/machines/machine-1/containers/container-123/stats", "reader-1")
	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the per-container stats route")
}

// TestContainerHandler_Stats_LiteralRouteWins pins that the literal
// /stats route beats the /{containerId} subtree: with both mounted the
// way app.go mounts them, GET /containers/stats reaches the all-stats
// handler (a swallowed route would fall into the {containerId} subtree
// and answer 404, not a stats envelope).
func TestContainerHandler_Stats_LiteralRouteWins(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return []define.ContainerStats{runningContainerStats()}, nil
		},
	}
	// inspectFn stays unarmed: a swallowed route would resolve the
	// "stats" container through Get and panic on the unarmed inspect.
	router := statsAccessRouter(machineRepo, accessRepo, newTestHandler(engine))

	w := statsRequestAsUser(router, http.MethodGet, "/api/machines/machine-1/containers/stats", "reader-1")

	require.Equal(t, http.StatusOK, w.Result().StatusCode, "the literal /stats route must win over the {containerId} subtree")
	var body ContainerStatsResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the all-stats handler must answer the list envelope")
	require.Len(t, body.Containers, 1)
	assert.Equal(t, "container-123", body.Containers[0].ID)
}
