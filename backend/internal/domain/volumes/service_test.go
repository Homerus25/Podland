package volumes

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/Homerus25/podland/backend/internal/domain/machines"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestVolumeService_List_ConnectionError pins the connection-failure path:
// a GetConnection failure must surface as a wrapped error and no result.
func TestVolumeService_List_ConnectionError(t *testing.T) {
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewVolumeService(mockConnManager)

	result, err := service.List(context.Background(), "test-user", "test-machine", entities.VolumeListOptions{})

	assert.Error(t, err, "should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "error should wrap the connection failure")
	assert.Nil(t, result, "should return nil result on connection error")
}

// TestVolumeService_List_ContextCancelled pins the cancelled-context path.
func TestVolumeService_List_ContextCancelled(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
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
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewVolumeService(mockConnManager)

	result, err := service.List(context.Background(), "test-user", "test-machine", entities.VolumeListOptions{})

	assert.Error(t, err, "engine call without a real podman client must fail")
	assert.Contains(t, err.Error(), "failed to list volumes", "engine failures must be wrapped consistently")
	assert.Nil(t, result)
}
