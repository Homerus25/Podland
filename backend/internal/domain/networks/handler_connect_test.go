package networks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"go.podman.io/common/libnetwork/types"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// connectRequest serves POST
// /api/machines/{id}/networks/{networkName}/connect with the given JSON
// body through the handler with an authenticated test identity.
func connectRequest(connect http.HandlerFunc, networkName, body string) *httptest.ResponseRecorder {
	return testutils.Perform(connect, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/networks/" + networkName + "/connect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "networkName": networkName},
		Body:      strings.NewReader(body),
	})
}

// TestNetworkHandler_Connect_Success pins the connect contract: 200
// with an empty body, and the URL-param network plus the body's
// container reaching the machine.
func TestNetworkHandler_Connect_Success(t *testing.T) {
	networks := []types.Network{
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
	}
	var connectedNetwork, connectedContainer string
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
		connectFn: func(networkName, containerID string) error {
			connectedNetwork = networkName
			connectedContainer = containerID
			return nil
		},
	}

	w := connectRequest(newTestHandler(engine).Connect, "frontend", `{"container":"ctr-a"}`)

	assert.Equal(t, 200, w.Code, "connect should return 200")
	assert.Equal(t, "frontend", connectedNetwork, "the URL-param network must reach the machine")
	assert.Equal(t, "ctr-a", connectedContainer, "the decoded container must reach the machine")
	assert.JSONEq(t, `{}`, strings.TrimSpace(w.Body.String()), "connect answers an empty body")
}

// TestNetworkHandler_Connect_MissingContainer pins the struct-tag
// branch of the decode ritual: a body without the required container
// answers 400 VALIDATION_FAILED before the engine is reached.
func TestNetworkHandler_Connect_MissingContainer(t *testing.T) {
	engine := &fakeNetworkEngine{} // listFn unarmed: validation must fail first

	w := connectRequest(newTestHandler(engine).Connect, "frontend", `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Connect_MalformedBody pins the decode branch: a
// body that is not valid JSON answers 400 INVALID_REQUEST.
func TestNetworkHandler_Connect_MalformedBody(t *testing.T) {
	engine := &fakeNetworkEngine{} // listFn unarmed: decoding must fail first

	w := connectRequest(newTestHandler(engine).Connect, "frontend", `not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Connect_UnknownNetwork pins the unknown-network
// contract through the HTTP surface: 404 NETWORK_NOT_FOUND.
func TestNetworkHandler_Connect_UnknownNetwork(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return []types.Network{
				networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
			}, nil
		},
		// connectFn stays unarmed: an unknown network must never reach
		// the connect call.
	}

	w := connectRequest(newTestHandler(engine).Connect, "ghost-net", `{"container":"ctr-a"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "NETWORK_NOT_FOUND", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Connect_UnknownContainer pins the unknown-container
// contract through the HTTP surface: 404 CONTAINER_NOT_FOUND.
func TestNetworkHandler_Connect_UnknownContainer(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return []types.Network{
				networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
			}, nil
		},
		connectFn: func(networkName, containerID string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such container",
			}
		},
	}

	w := connectRequest(newTestHandler(engine).Connect, "frontend", `{"container":"ghost-ctr"}`)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "CONTAINER_NOT_FOUND", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Connect_AcquisitionFailure pins the acquisition-failure
// branch through the HTTP surface: the provider's CONNECTION_FAILED
// sentinel crosses the service untouched and the envelope answers its
// 502 status.
func TestNetworkHandler_Connect_AcquisitionFailure(t *testing.T) {
	handler := NewNetworkHandler(NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}))

	w := connectRequest(handler.Connect, "frontend", `{"container":"ctr-a"}`)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"an acquisition failure must surface as the 502 the CONNECTION_FAILED sentinel carries")
}

// TestNetworkHandler_Connect_EngineError pins the engine-failure mapping
// through the HTTP surface: the service wraps the engine's error with
// domain context, so it is not an *AppError and the envelope answers
// the generic 500 fallback the doku lists for the endpoint.
func TestNetworkHandler_Connect_EngineError(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return []types.Network{
				networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16"),
			}, nil
		},
		connectFn: func(networkName, containerID string) error {
			return assert.AnError
		},
	}

	w := connectRequest(newTestHandler(engine).Connect, "frontend", `{"container":"ctr-a"}`)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "an engine failure must surface as a 500")
	assert.Equal(t, "INTERNAL_ERROR", testutils.DecodeErrorEnvelope(t, resp).Error.Code,
		"the envelope maps the service's wrapped engine error to the generic internal-error code")
}
