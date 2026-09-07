package volumes

import (
	"context"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Connection-failure mapping (502 CONNECTION_FAILED) is owned by the
// connections.Manager seam and pinned once in the connections package
// tests — not re-pinned per service method here.

// TestVolumeService_List_ContextCancelled pins the cancelled-context path.
func TestVolumeService_List_ContextCancelled(t *testing.T) {
	var mockConnManager connections.Manager = testutils.NewMockConnectionManager()
	service := NewVolumeService(mockConnManager)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := service.List(ctx, "test-user", "test-machine", entities.VolumeListOptions{})

	assert.Error(t, err, "should return error when context is cancelled")
	assert.Nil(t, result, "should return nil result on context cancellation")
}

// TestVolumeService_List_EngineError pins that an engine failure surfaces
// wrapped as "failed to list volumes". The mock connection carries no real
// podman client, so the tunnel call errors; the happy path is covered by
// the integration suite.
func TestVolumeService_List_EngineError(t *testing.T) {
	var mockConnManager connections.Manager = testutils.NewMockConnectionManager()
	service := NewVolumeService(mockConnManager)

	result, err := service.List(context.Background(), "test-user", "test-machine", entities.VolumeListOptions{})

	assert.Error(t, err, "engine call without a real podman client must fail")
	assert.Contains(t, err.Error(), "failed to list volumes", "engine failures must be wrapped consistently")
	assert.Nil(t, result)
}

// TestVolumeService_Get_EngineError pins that an engine failure surfaces
// wrapped as "failed to inspect volume". The mock connection carries no real
// podman client, so the tunnel call errors; happy path and not-found mapping
// are covered by the integration suite.
func TestVolumeService_Get_EngineError(t *testing.T) {
	var mockConnManager connections.Manager = testutils.NewMockConnectionManager()
	service := NewVolumeService(mockConnManager)

	result, err := service.Get(context.Background(), "test-user", "test-machine", "my-volume")

	assert.Error(t, err, "engine call without a real podman client must fail")
	assert.Contains(t, err.Error(), "failed to inspect volume", "engine failures must be wrapped consistently")
	assert.Nil(t, result)
}

// TestVolumeService_Create_EngineError pins that an engine failure surfaces
// wrapped as "failed to create volume". The mock connection carries no real
// podman client, so the tunnel call errors; happy paths are covered by the
// integration suite.
func TestVolumeService_Create_EngineError(t *testing.T) {
	var mockConnManager connections.Manager = testutils.NewMockConnectionManager()
	service := NewVolumeService(mockConnManager)

	result, err := service.Create(context.Background(), "test-user", "test-machine", VolumeCreateRequest{})

	assert.Error(t, err, "engine call without a real podman client must fail")
	assert.Contains(t, err.Error(), "failed to create volume", "engine failures must be wrapped consistently")
	assert.Nil(t, result)
}

// TestMapCreateError pins the client-error mapping for create: podman 4xx
// ErrorModels become INVALID_REQUEST carrying podman's message, while 5xx
// and unrelated errors stay unmapped (wrapped 5xx at the handler).
func TestMapCreateError(t *testing.T) {
	appErr := mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusBadRequest, Message: "volume name invalid"})
	require.NotNil(t, appErr, "podman 400 must map to a client error")
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Equal(t, "volume name invalid", appErr.Message)

	// The adapter excludes the 404/409 answers from its client-error
	// predicate; volumes create folds them back in, so the duplicate-name
	// 409 stays the 400 INVALID_REQUEST the create contract documents.
	appErr = mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: `volume with name "x" already exists`})
	require.NotNil(t, appErr, "podman 409 duplicate name must still map to a client error")
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)

	assert.Nil(t, mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusInternalServerError}), "podman 5xx must not map to a client error")
	assert.Nil(t, mapCreateError(assert.AnError), "unrelated errors must not map to a client error")
}
