package containers

import (
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The create mapping pins live in create_spec_test.go as direct
// builder table tests (the pure per-domain create builder settlement,
// backend-conventions). This suite keeps only the classification pins
// through the service over the fake engine: the unparseable-memory
// VALIDATION_FAILED crossing the service, and the thin-lease ordering
// that keeps the failure lease-free (the pods twin's
// service_create_test.go is the reference).

// TestContainerService_Create_UnparseableMemoryAnswersValidationFailed
// pins the documented 400 crossing the service: an unparseable memory
// value is a VALIDATION_FAILED, not a 500.
func TestContainerService_Create_UnparseableMemoryAnswersValidationFailed(t *testing.T) {
	engine := &fakeContainerEngine{}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "machine", ContainerCreateRequest{Image: "nginx:latest", Memory: "not-a-size"})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "an unparseable memory value must answer an AppError")
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
}

// TestContainerService_Create_UnparseableMemorySkipsEngineLease pins
// the thin-lease ordering: the spec is built before the lease is
// acquired, so a request-local validation failure (the unparseable
// memory) never costs an engine lease — the provider seam is not
// crossed at all.
func TestContainerService_Create_UnparseableMemorySkipsEngineLease(t *testing.T) {
	engine := &fakeContainerEngine{}
	crossed := false
	provider := fakeEngineProvider{engine: engine, opRecorder: func(op string) { crossed = true }}
	service := NewContainerService(provider)

	_, err := service.Create(t.Context(), "user", "machine", ContainerCreateRequest{Image: "nginx:latest", Memory: "not-a-size"})

	require.Error(t, err, "an unparseable memory value must answer the builder's error")
	assert.False(t, crossed, "a request-local validation failure must not acquire the engine lease")
}
