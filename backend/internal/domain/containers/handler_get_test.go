package containers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestContainerHandler_Get_MissingContainerAnswers404 pins the documented
// 404 envelope end to end: the real service classifies the tunnel's
// per-item not-found answer and the web envelope maps the sentinel.
func TestContainerHandler_Get_MissingContainerAnswers404(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such container %q", id)}, nil
		},
	}

	req := httptest.NewRequest("GET", "/api/machines/test-machine/containers/container-123", nil).
		WithContext(authedContext())

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/machines/{id}/containers/{containerId}", newTestHandler(engine).Get)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "a missing container must answer 404")
	var body appErrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test-request-id", body.RequestID)
	assert.Equal(t, "CONTAINER_NOT_FOUND", body.Error.Code)
	assert.Equal(t, "Container not found", body.Error.Message)
}
