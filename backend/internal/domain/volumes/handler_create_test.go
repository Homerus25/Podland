package volumes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

func TestVolumeHandler_Create_Success(t *testing.T) {
	cfg := namedVolumeFixture("2026-08-30T12:00:00Z")
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}
	req := VolumeCreateRequest{Name: "my-volume", Labels: map[string]string{"app": "test"}, Options: map[string]string{"type": "tmpfs"}}

	mockService := new(MockVolumeService)
	mockService.On("Create", mock.Anything, "test-user", "test-machine", req).Return(report, nil)
	handler := NewVolumeHandler(mockService)

	w := recordCreateRequest(handler.Create, "test-machine", `{"name":"my-volume","labels":{"app":"test"},"options":{"type":"tmpfs"}}`)

	assert.Equal(t, 201, w.Code, "create should return 201")
	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a volume config")
	assert.Equal(t, "my-volume", got.Name)
	assert.Equal(t, "local", got.Driver)
	assert.Equal(t, "2026-08-30T12:00:00Z", got.CreatedAt)
}

func TestVolumeHandler_Create_Anonymous(t *testing.T) {
	cfg := anonymousVolumeFixture()
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}
	// Empty name is forwarded as-is: podman generates the name.
	req := VolumeCreateRequest{}

	mockService := new(MockVolumeService)
	mockService.On("Create", mock.Anything, "test-user", "test-machine", req).Return(report, nil)
	handler := NewVolumeHandler(mockService)

	w := recordCreateRequest(handler.Create, "test-machine", `{}`)

	assert.Equal(t, 201, w.Code)
	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Anonymous, "anonymous volume must be flagged")
	assert.NotEmpty(t, got.Name, "generated name must be present in the response")
}

func TestVolumeHandler_Create_MalformedBody(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))

	w := recordCreateRequest(handler.Create, "test-machine", `{"labels": 42}`)

	assert.Equal(t, 400, w.Code, "wrong-typed body must be rejected as a validation error")
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
}

func TestVolumeHandler_Create_PodmanRejectedName(t *testing.T) {
	// Podman's rejection maps to a 4xx app error inside the service;
	// the handler must relay it in the standard envelope, not 5xx.
	mockService := new(MockVolumeService)
	mockService.On("Create", mock.Anything, "test-user", "test-machine", VolumeCreateRequest{Name: "bad name"}).
		Return((*entities.VolumeInspectReport)(nil), errors.ErrInvalidRequest.WithMessage("invalid volume name"))
	handler := NewVolumeHandler(mockService)

	w := recordCreateRequest(handler.Create, "test-machine", `{"name":"bad name"}`)

	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
	assert.Contains(t, w.Body.String(), "requestId")
}

func TestVolumeHandler_Create_ServiceError(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Create", mock.Anything, "test-user", "test-machine", VolumeCreateRequest{}).
		Return((*entities.VolumeInspectReport)(nil), assert.AnError)
	handler := NewVolumeHandler(mockService)

	w := recordCreateRequest(handler.Create, "test-machine", `{}`)

	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), "requestId")
}

// recordCreateRequest serves POST /api/machines/{id}/volumes through the
// given handler with an authenticated test identity.
func recordCreateRequest(create http.HandlerFunc, machineID, body string) *httptest.ResponseRecorder {
	ctx := contextutils.SetUserID(contextutils.SetRequestID(context.Background(), "test-request-id"), "test-user")
	req := httptest.NewRequest(http.MethodPost, "/api/machines/"+machineID+"/volumes", strings.NewReader(body)).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	create(w, req)
	return w
}

func TestVolumeHandler_Create_MalformedJSON(t *testing.T) {
	handler := NewVolumeHandler(new(MockVolumeService))

	w := recordCreateRequest(handler.Create, "test-machine", `{bad`)

	assert.Equal(t, 400, w.Code, "syntactically malformed body must be rejected as a validation error")
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
}
