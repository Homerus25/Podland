package networks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/common/libnetwork/types"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// createRequest serves POST /api/hosts/{id}/networks with the given
// JSON body through the handler with an authenticated test identity.
func createRequest(create http.HandlerFunc, body string) *httptest.ResponseRecorder {
	return testutils.Perform(create, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/networks",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
		Body:      strings.NewReader(body),
	})
}

// TestNetworkHandler_Create_Success pins the create contract: 201 with
// the created network as the list DTO, and the listed containers
// attached in the same call.
func TestNetworkHandler_Create_Success(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	var createdName string
	var attached []string
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			createdName = name
			return &created, nil
		},
		connectFn: func(networkName, containerID string) error {
			attached = append(attached, containerID)
			return nil
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"frontend","containers":["ctr-a","ctr-b"]}`)

	assert.Equal(t, 201, w.Code, "create should return 201")
	assert.Equal(t, "frontend", createdName, "the decoded request must reach the host")
	assert.Equal(t, []string{"ctr-a", "ctr-b"}, attached, "the listed containers must be attached in the same call")
	var got NetworkResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a network")
	assert.Equal(t, "frontend", got.Name)
	assert.Equal(t, "bridge", got.Driver)
	assert.Equal(t, []string{"10.89.0.0/16"}, got.Subnets)
	assert.Equal(t, "2026-10-08T10:00:00Z", got.CreatedAt)
}

// TestNetworkHandler_Create_MissingName pins the struct-tag branch of
// the decode ritual: a body without the required name answers 400
// VALIDATION_FAILED before the engine is reached.
func TestNetworkHandler_Create_MissingName(t *testing.T) {
	engine := &fakeNetworkEngine{} // createFn unarmed: validation must fail first

	w := createRequest(newTestHandler(engine).Create, `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_FAILED", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Create_MalformedBody pins the decode branch: a
// body that is not valid JSON answers 400 INVALID_REQUEST.
func TestNetworkHandler_Create_MalformedBody(t *testing.T) {
	engine := &fakeNetworkEngine{} // createFn unarmed: decoding must fail first

	w := createRequest(newTestHandler(engine).Create, `not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_REQUEST", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Create_DuplicateName pins the duplicate-name
// contract through the HTTP surface: 409 NETWORK_ALREADY_EXISTS.
func TestNetworkHandler_Create_DuplicateName(t *testing.T) {
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusConflict,
				Message:      "network already exists",
			}
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"frontend"}`)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "NETWORK_ALREADY_EXISTS", testutils.DecodeErrorEnvelope(t, w.Result()).Error.Code)
}

// TestNetworkHandler_Create_AttachFailureNamesContainer pins the
// attach-failure contract through the HTTP surface: 409
// NETWORK_ATTACH_FAILED whose message names the failing container.
func TestNetworkHandler_Create_AttachFailureNamesContainer(t *testing.T) {
	created := networkFixture("frontend", "bridge", "2026-10-08T10:00:00Z", "10.89.0.0/16")
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return &created, nil
		},
		connectFn: func(networkName, containerID string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      "container is not running",
			}
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"frontend","containers":["bad-ctr"]}`)

	assert.Equal(t, http.StatusConflict, w.Code)
	envelope := testutils.DecodeErrorEnvelope(t, w.Result())
	assert.Equal(t, "NETWORK_ATTACH_FAILED", envelope.Error.Code)
	assert.Contains(t, envelope.Error.Message, "bad-ctr", "the envelope error must name the failing container")
}

// TestNetworkHandler_Create_AcquisitionFailure pins the acquisition-failure
// branch through the HTTP surface: the provider's CONNECTION_FAILED
// sentinel crosses the service untouched and the envelope answers its
// 502 status.
func TestNetworkHandler_Create_AcquisitionFailure(t *testing.T) {
	handler := NewNetworkHandler(NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}))

	w := createRequest(handler.Create, `{"name":"frontend"}`)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode,
		"an acquisition failure must surface as the 502 the CONNECTION_FAILED sentinel carries")
}

// TestNetworkHandler_Create_EngineError pins the engine-failure mapping
// through the HTTP surface: the service wraps the engine's error with
// domain context, so it is not an *AppError and the envelope answers
// the generic 500 fallback the doku lists for the endpoint.
func TestNetworkHandler_Create_EngineError(t *testing.T) {
	engine := &fakeNetworkEngine{
		createFn: func(name string) (*types.Network, error) {
			return nil, assert.AnError
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"frontend"}`)
	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "an engine failure must surface as a 500")
	assert.Equal(t, "INTERNAL_ERROR", testutils.DecodeErrorEnvelope(t, resp).Error.Code,
		"the envelope maps the service's wrapped engine error to the generic internal-error code")
}
