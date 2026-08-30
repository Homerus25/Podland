package volumes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestVolumeHandler_List_Success pins the list contract: 200, JSON array,
// and the DTO fields the API promises (name, driver, createdAt, anonymous,
// labels, options, mountpoint, size when reported).
func TestVolumeHandler_List_Success(t *testing.T) {
	createdAt := "2026-08-30T10:00:00Z"
	reports := []*entities.VolumeListReport{
		{VolumeConfigResponse: namedVolumeFixture(createdAt)},
		{VolumeConfigResponse: anonymousVolumeFixture()},
	}

	mockService := new(testutils.MockVolumeService)
	mockService.On("List", mock.Anything, "test-user", "test-machine", mock.Anything).Return(reports, nil)
	handler := NewVolumeHandler(mockService)

	w := recordListRequest(handler.List, "test-machine")

	assert.Equal(t, 200, w.Code, "list should return 200")

	volumes := decodeVolumes(t, w.Body.String())
	assert.Len(t, volumes, 2, "named and anonymous volumes must both appear")

	named := volumes[0]
	assert.Equal(t, "my-volume", named.Name)
	assert.Equal(t, "local", named.Driver)
	assert.Equal(t, createdAt, named.CreatedAt)
	assert.False(t, named.Anonymous)
	assert.Equal(t, "/var/lib/containers/storage/volumes/my-volume/_data", named.Mountpoint)
	assert.Equal(t, map[string]string{"app": "test"}, named.Labels)
	assert.Equal(t, map[string]string{"type": "tmpfs"}, named.Options)
	assert.NotContains(t, w.Body.String(), `"size":`,
		"size must be omitted when podman does not report it")

	anonymous := volumes[1]
	assert.True(t, anonymous.Anonymous, "anonymous volumes must be flagged")
	assert.NotEmpty(t, anonymous.Name)
}

func TestVolumeHandler_List_ConnectionError(t *testing.T) {
	mockService := new(testutils.MockVolumeService)
	mockService.On("List", mock.Anything, "test-user", "test-machine", mock.Anything).
		Return([]*entities.VolumeListReport(nil), assert.AnError)
	handler := NewVolumeHandler(mockService)

	w := recordListRequest(handler.List, "test-machine")

	assert.Equal(t, 500, w.Code, "connection failures surface as 5xx")
	assert.Contains(t, w.Body.String(), "requestId", "error envelope must carry requestId")
}

func TestVolumeHandler_List_MissingUserID(t *testing.T) {
	handler := NewVolumeHandler(new(testutils.MockVolumeService))
	w := recordListRequestAnonymous(handler.List, "test-machine")
	assert.Equal(t, 401, w.Code, "missing user identity must be rejected")
}

// recordListRequest serves a GET /api/machines/{id}/volumes through the
// given handler with an authenticated test identity in the context.
func recordListRequest(list http.HandlerFunc, machineID string) *httptest.ResponseRecorder {
	ctx := contextutils.SetUserID(contextutils.SetRequestID(context.Background(), "test-request-id"), "test-user")
	req := httptest.NewRequest(http.MethodGet, "/api/machines/"+machineID+"/volumes", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	list(w, req)
	return w
}

// recordListRequestAnonymous serves the same request without a user ID
// in the context.
func recordListRequestAnonymous(list http.HandlerFunc, machineID string) *httptest.ResponseRecorder {
	ctx := contextutils.SetRequestID(context.Background(), "test-request-id")
	req := httptest.NewRequest(http.MethodGet, "/api/machines/"+machineID+"/volumes", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	list(w, req)
	return w
}

func decodeVolumes(t *testing.T, body string) []VolumeResponse {
	t.Helper()
	var out []VolumeResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out), "response must decode as a JSON array of volumes")
	return out
}
