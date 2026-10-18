package networks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// getRequest serves GET
// /api/machines/{id}/networks/{networkName} through the handler with
// an authenticated test identity.
func getRequest(get http.HandlerFunc, networkName string) *httptest.ResponseRecorder {
	return testutils.Perform(get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/networks/" + networkName,
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "networkName": networkName},
	})
}

// TestNetworkHandler_Get_Success pins the detail contract: 200 with
// the mapped NetworkDetailResponse as the top-level JSON body — name,
// id, driver, createdAt (RFC3339), subnets, and the attached
// containers resolved from the inspect report — and the URL-param
// network reaching the machine.
func TestNetworkHandler_Get_Success(t *testing.T) {
	report := networkInspectFixture("frontend")
	var inspectedName string
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			inspectedName = name
			return []entities.NetworkInspectReport{report}, nil, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "frontend")

	require.Equal(t, http.StatusOK, w.Code, "get should return 200")
	assert.Equal(t, "frontend", inspectedName, "the URL-param network must reach the machine")

	var body NetworkDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "the top-level body must be the detail DTO")
	assert.Equal(t, NetworkDetailResponse{
		Name:      "frontend",
		ID:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		Driver:    "bridge",
		CreatedAt: "2026-09-01T12:30:00Z",
		Subnets:   []string{},
		Containers: []NetworkContainerRef{
			{ID: "ctr-a", Name: "web"},
		},
	}, body)
}

// TestNetworkHandler_Get_MissingNetworkAnswers404 pins the
// documented 404 envelope end to end: the real service classifies the
// tunnel's per-item not-found answer and the web envelope maps the
// sentinel.
func TestNetworkHandler_Get_MissingNetworkAnswers404(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such network %q", name)}, nil
		},
	}

	w := getRequest(newTestHandler(engine).Get, "ghost-net")

	require.Equal(t, http.StatusNotFound, w.Code, "a missing network must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "NETWORK_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Network not found", body.Error.Message)
}

// TestNetworkHandler_Get_MissingNetworkName pins the URL-param guard:
// a request without the networkName route param answers 400
// MISSING_NETWORK_NAME before the engine is reached.
func TestNetworkHandler_Get_MissingNetworkName(t *testing.T) {
	engine := &fakeNetworkEngine{} // inspectFn unarmed: the guard must fail first

	w := testutils.Perform(newTestHandler(engine).Get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/test-machine/networks/",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "MISSING_NETWORK_NAME", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// networksDetailAccessRouter mirrors app.go's mounting of the
// networks subtree: GET /api/machines/{id}/networks/{networkName} and
// GET .../networks/{networkName}/inspect both ride the
// RequireMachineAccess subtree at read level (no requireWrite), side
// by side without shadowing each other.
func networksDetailAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service *NetworkService) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			r.Route("/networks", func(r chi.Router) {
				r.Get("/{networkName}", NewNetworkHandler(service).Get)
				r.Get("/{networkName}/inspect", NewNetworkHandler(service).Inspect)
			})
		})
	})
	return r
}

// TestNetworkHandler_Get_ReadLevelGranteeReachesRoute pins the route
// mounting: GET /api/machines/{id}/networks/{networkName} rides the
// RequireMachineAccess subtree at read level (no requireWrite), so a
// read-level grantee reaches the handler.
func TestNetworkHandler_Get_ReadLevelGranteeReachesRoute(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			return []entities.NetworkInspectReport{networkInspectFixture("frontend")}, nil, nil
		},
	}
	router := networksDetailAccessRouter(machineRepo, accessRepo, NewNetworkService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/networks/frontend", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the detail endpoint")
}

// TestNetworkHandler_Get_DoesNotShadowInspectRoute pins the route
// pair's coexistence: with both GET /{networkName} and
// GET /{networkName}/inspect mounted on the same subtree, the inspect
// route still answers its own requests.
func TestNetworkHandler_Get_DoesNotShadowInspectRoute(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			return []entities.NetworkInspectReport{networkInspectFixture("frontend")}, nil, nil
		},
	}
	router := networksDetailAccessRouter(machineRepo, accessRepo, NewNetworkService(fakeEngineProvider{engine: engine}))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/networks/frontend/inspect", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "the inspect route must keep answering beside the detail route")
}
