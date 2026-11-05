package pods

import (
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// The create mapping pins live in create_spec_test.go as direct
// builder table tests (the pure per-domain create builder settlement,
// backend-conventions). This suite keeps only the classification pins
// through the service over the fake engine: the unparseable-memory
// VALIDATION_FAILED crossing the service, and the podman-rejection
// INVALID_REQUEST.

// TestPodService_Create_UnparseableMemoryAnswersValidationFailed pins
// the documented 400 crossing the service: an unparseable memory value
// is a VALIDATION_FAILED, not a 500.
func TestPodService_Create_UnparseableMemoryAnswersValidationFailed(t *testing.T) {
	engine := &fakePodEngine{}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "host", PodCreateRequest{Name: "web-pod", Memory: "not-a-size"})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "an unparseable memory value must answer an AppError")
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
}

// TestPodService_Create_UnparseableMemorySkipsEngineLease pins the
// thin-lease ordering: the spec is built before the lease is acquired,
// so a request-local validation failure (the unparseable memory) never
// costs an engine lease — the provider seam is not crossed at all.
func TestPodService_Create_UnparseableMemorySkipsEngineLease(t *testing.T) {
	engine := &fakePodEngine{}
	crossed := false
	provider := fakeEngineProvider{engine: engine, opRecorder: func(op string) { crossed = true }}
	service := NewPodService(provider)

	_, err := service.Create(t.Context(), "user", "host", PodCreateRequest{Name: "web-pod", Memory: "not-a-size"})

	require.Error(t, err, "an unparseable memory value must answer the builder's error")
	assert.False(t, crossed, "a request-local validation failure must not acquire the engine lease")
}

// TestPodService_Create_PodmanClientRejectionAnswersInvalidRequest pins
// the create classification: a podman-side 4xx rejection (duplicate name,
// invalid name format) surfaces as 400 INVALID_REQUEST carrying podman's
// message.
func TestPodService_Create_PodmanClientRejectionAnswersInvalidRequest(t *testing.T) {
	engine := &fakePodEngine{
		createFn: func(entities.PodSpec) (*entities.PodCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: 409, Because: `name "web-pod" is in use`}
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	_, err := service.Create(t.Context(), "user", "host", PodCreateRequest{Name: "web-pod"})

	require.Error(t, err)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "podman-side 4xx rejections must map to an AppError")
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.NotEmpty(t, appErr.Message, "the message must carry podman's refusal text")
}
