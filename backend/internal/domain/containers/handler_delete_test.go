package containers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

func newTestHandler(engine *fakeContainerEngine) *ContainerHandler {
	return NewContainerHandler(NewContainerService(fakeEngineProvider{engine: engine}))
}

func authedContext() context.Context {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	return contextutils.SetRequestID(ctx, "test-request-id")
}

// TestContainerHandler_Delete_Success tests successful container deletion,
// with the force flag reaching the machine parsed.
func TestContainerHandler_Delete_Success(t *testing.T) {
	var gotID string
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) ([]*reports.RmReport, error) {
			gotID, gotForce = id, force
			return []*reports.RmReport{{Id: id}}, nil
		},
	}

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", newTestHandler(engine).Delete)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success")
	assert.Equal(t, "container-123", gotID)
	assert.False(t, gotForce, "a missing force param must delete unforced")
}

// TestContainerHandler_Delete_Force tests deletion with force query parameter
func TestContainerHandler_Delete_Force(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) ([]*reports.RmReport, error) {
			gotForce = force
			return []*reports.RmReport{{Id: id}}, nil
		},
	}

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/containers/container-123?force=true", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Delete("/api/machines/{id}/containers/{containerId}", newTestHandler(engine).Delete)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success with force=true")
	assert.True(t, gotForce, "force=true must reach the machine")
}
