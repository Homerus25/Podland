package images

import (
	"context"
	"encoding/json"
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

// TestImageHandler_Prune_Success pins the prune contract: 200 with one
// report entry per removed image, id and reclaimed size mapped, and
// per-image errors carried in the entry.
func TestImageHandler_Prune_Success(t *testing.T) {
	prunes := []*reports.PruneReport{
		{Id: "sha256:aaa", Size: 4096},
		{Id: "sha256:bbb", Size: 0, Err: fmt.Errorf("image is used by container")},
	}

	mockService := new(MockImageService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return(prunes, nil)
	handler := NewImageHandler(mockService, nil)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 200, w.Code, "prune should return 200")
	var got []ImagePruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed image must have a report entry")
	assert.Equal(t, "sha256:aaa", got[0].ID)
	assert.Equal(t, uint64(4096), got[0].Size)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "sha256:bbb", got[1].ID)
	assert.Equal(t, "image is used by container", got[1].Error, "per-image error must be carried")
}

// TestImageHandler_Prune_EmptyReport pins that pruning with nothing to
// remove still returns 200 with an empty JSON array (not null).
func TestImageHandler_Prune_EmptyReport(t *testing.T) {
	mockService := new(MockImageService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return([]*reports.PruneReport{}, nil)
	handler := NewImageHandler(mockService, nil)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 200, w.Code)
	var got []ImagePruneResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}

func TestImageHandler_Prune_ServiceError(t *testing.T) {
	mockService := new(MockImageService)
	mockService.On("Prune", mock.Anything, "test-user", "test-machine").Return(nil, fmt.Errorf("boom"))
	handler := NewImageHandler(mockService, nil)

	w := recordPruneRequest(handler.Prune, "test-machine")

	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), "requestId")
}

// TestFromImagePruneReports_NilEntries pins that nil reports are skipped
// rather than marshalled as empty entries.
func TestFromImagePruneReports_NilEntries(t *testing.T) {
	got := FromImagePruneReports([]*reports.PruneReport{nil, {Id: "sha256:x", Size: 1}})
	require.Len(t, got, 1)
	assert.Equal(t, "sha256:x", got[0].ID)
}

// recordPruneRequest serves POST /api/machines/{id}/images/prune
// through the given handler with an authenticated test identity.
func recordPruneRequest(prune http.HandlerFunc, machineID string) *httptest.ResponseRecorder {
	ctx := contextutils.SetUserID(contextutils.SetRequestID(context.Background(), "test-request-id"), "test-user")
	req := httptest.NewRequest(http.MethodPost, "/api/machines/"+machineID+"/images/prune", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	prune(w, req)
	return w
}
