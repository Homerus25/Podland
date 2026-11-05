package networks

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/common/libnetwork/types"
)

// listRequest performs a GET /api/hosts/{id}/networks through the
// handler with an authenticated test identity.
func listRequest(list http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(list, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/networks",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})
}

// TestNetworkHandler_List_Success pins the list contract: 200, JSON
// array, and the DTO fields the API promises (name, driver, subnets as
// CIDR strings, createdAt).
func TestNetworkHandler_List_Success(t *testing.T) {
	networks := []types.Network{
		networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16", "fd00:10:89::/64"),
		networkFixture("host-net", "host", "2026-09-02T08:00:00Z"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
	}

	w := listRequest(newTestHandler(engine).List)

	assert.Equal(t, 200, w.Code, "list should return 200")

	networksOut := decodeNetworks(t, w.Body.String())
	require.Len(t, networksOut, 3, "every network podman reports must appear")

	defaultBridge := networksOut[0]
	assert.Equal(t, "podman", defaultBridge.Name)
	assert.Equal(t, "bridge", defaultBridge.Driver)
	assert.Equal(t, []string{"10.88.0.0/16"}, defaultBridge.Subnets)
	assert.Equal(t, "2026-08-30T10:00:00Z", defaultBridge.CreatedAt)

	multiSubnet := networksOut[1]
	assert.Equal(t, "frontend", multiSubnet.Name)
	assert.Equal(t, []string{"10.89.0.0/16", "fd00:10:89::/64"}, multiSubnet.Subnets,
		"every subnet must be carried as a CIDR string")

	subnetless := networksOut[2]
	assert.Equal(t, "host-net", subnetless.Name)
	assert.Equal(t, "host", subnetless.Driver)
	assert.Equal(t, []string{}, subnetless.Subnets, "a network without subnets must answer an empty array, not null")
}

// TestNetworkHandler_List_Empty pins the empty-list contract: 200 with
// an empty JSON array, never null.
func TestNetworkHandler_List_Empty(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return nil, nil
		},
	}

	w := listRequest(newTestHandler(engine).List)

	assert.Equal(t, 200, w.Code, "list should return 200")
	assert.JSONEq(t, `[]`, w.Body.String(), "an empty list must answer an empty JSON array")
}

// TestNetworkHandler_List_EngineError pins the engine-failure mapping
// through the HTTP surface: the service wraps the engine's error with
// domain context, so it is not an *AppError and the envelope answers
// the generic 500 fallback the doku lists for the endpoint.
func TestNetworkHandler_List_EngineError(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return nil, errors.New("podman network list exploded")
		},
	}

	w := listRequest(newTestHandler(engine).List)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "an engine failure must surface as a 500")
	assert.Equal(t, "INTERNAL_ERROR", testutils.DecodeErrorEnvelope(t, resp).Error.Code,
		"the envelope maps the service's wrapped engine error to the generic internal-error code")
}

// TestNetworkHandler_List_AcquisitionFailure pins the acquisition-failure
// branch through the HTTP surface: the provider's CONNECTION_FAILED
// sentinel crosses the service untouched (pinned at the service seam in
// service_test.go) and the envelope answers its 502 status. The body is
// the envelope's generic 5xx rewrite — the status line is the
// wire-distinguishing part, pinned once in internal/web — so this test
// pins the status, not the rewrite.
func TestNetworkHandler_List_AcquisitionFailure(t *testing.T) {
	handler := NewNetworkHandler(NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}))

	w := listRequest(handler.List)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"an acquisition failure must surface as the 502 the CONNECTION_FAILED sentinel carries")
}

func decodeNetworks(t *testing.T, body string) []NetworkResponse {
	t.Helper()
	var out []NetworkResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out), "response must decode as a JSON array of networks")
	return out
}
