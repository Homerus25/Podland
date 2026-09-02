package volumes

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// TestVolumeHandler_Prune_Success pins the prune contract: 200 with one
// report entry per removed volume, name and reclaimed size mapped, and
// per-volume errors carried in the entry.
func TestVolumeHandler_Prune_Success(t *testing.T) {
	prunes := []*reports.PruneReport{
		{Id: "unused-a", Size: 4096},
		{Id: "unused-b", Size: 0, Err: fmt.Errorf("device or resource busy")},
	}

	mockService := new(MockVolumeService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return(prunes, nil)
	handler := NewVolumeHandler(mockService)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 200, w.Code, "prune should return 200")
	var got []VolumePruneResponse
	require.NoError(t, jsonUnmarshalSlice(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed volume must have a report entry")
	assert.Equal(t, "unused-a", got[0].Name)
	assert.Equal(t, uint64(4096), got[0].Size)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "unused-b", got[1].Name)
	assert.Equal(t, "device or resource busy", got[1].Error, "per-volume error must be carried")
}

// TestVolumeHandler_Prune_EmptyReport pins that pruning with nothing to
// remove still returns 200 with an empty JSON array (not nil/null).
func TestVolumeHandler_Prune_EmptyReport(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return([]*reports.PruneReport{}, nil)
	handler := NewVolumeHandler(mockService)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 200, w.Code)
	var got []VolumePruneResponse
	require.NoError(t, jsonUnmarshalSlice(w.Body.Bytes(), &got))
	assert.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}

func TestVolumeHandler_Prune_ServiceError(t *testing.T) {
	mockService := new(MockVolumeService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return(nil, fmt.Errorf("boom"))
	handler := NewVolumeHandler(mockService)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), "requestId")
}

// TestFromPodmanPruneReports_NilEntries pins that nil reports are skipped
// rather than marshalled as empty entries.
func TestFromPodmanPruneReports_NilEntries(t *testing.T) {
	got := FromPodmanPruneReports([]*reports.PruneReport{nil, {Id: "x", Size: 1}})
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].Name)
}

// recordPruneRequest serves POST /api/machines/{id}/volumes/prune
// through the given handler with an authenticated test identity.
func recordPruneRequest(prune http.HandlerFunc, machineID string) *httptest.ResponseRecorder {
	ctx := contextutils.SetUserID(contextutils.SetRequestID(context.Background(), "test-request-id"), "test-user")
	req := httptest.NewRequest(http.MethodPost, "/api/machines/"+machineID+"/volumes/prune", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	prune(w, req)
	return w
}
