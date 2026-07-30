package images

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"github.com/yourorg/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestImageService_ImplementsInterface is a compile-time check that *ImageService
// satisfies ImageServiceInterface.
func TestImageService_ImplementsInterface(t *testing.T) {
	var _ ImageServiceInterface = (*ImageService)(nil)
}

// TestNewImageService_WithMockConnManager verifies that ImageService can be
// constructed with a mock ConnectionManagerInterface, enabling unit testing.
func TestNewImageService_WithMockConnManager(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()

	service := NewImageService(mockConnManager)

	assert.NotNil(t, service)
	assert.NotNil(t, service.connManager)
}

// TestNewImageService_WithErrorMockConnManager verifies the service can be
// constructed with a mock that simulates connection errors.
func TestNewImageService_WithErrorMockConnManager(t *testing.T) {
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(
		errors.New("simulated connection failure"),
	)

	service := NewImageService(mockConnManager)

	assert.NotNil(t, service)
}

// TestConcreteConnectionManager_ImplementsInterface is a compile-time check
// that concreteConnectionManager satisfies ConnectionManagerInterface.
func TestConcreteConnectionManager_ImplementsInterface(t *testing.T) {
	var _ machines.ConnectionManagerInterface = (*concreteConnectionManager)(nil)
}

// TestImageService_StubsReturnNotImplemented verifies the remaining stub methods
// (Delete, Pull) return ErrNotImplemented until real implementations land in slices 2–3.
func TestImageService_StubsReturnNotImplemented(t *testing.T) {
	service := NewImageService(testutils.NewMockConnectionManager())

	t.Run("Delete", func(t *testing.T) {
		err := service.Delete(nil, "user", "machine", "img", false)
		assert.Error(t, err)
	})

	t.Run("Pull", func(t *testing.T) {
		result, err := service.Pull(nil, "user", "machine", "nginx:latest", entities.ImagePullOptions{})
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

// TestImageService_List_ConnectionFailure tests that List properly wraps
// connection errors from the ConnectionManager.
func TestImageService_List_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewImageService(mockConnManager)

	result, err := service.List(ctx, "test-user", "test-machine", entities.ImageListOptions{})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

// TestImageService_List_InvocationShape tests that List follows the correct
// invocation shape: gets a connection, creates an ImageEngine, and calls List.
// With the mock connection manager (no real podman backend), the engine call
// fails — but we can verify the error is from the list step, not the connection step.
func TestImageService_List_InvocationShape(t *testing.T) {
	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewImageService(mockConnManager)

	result, err := service.List(ctx, "test-user", "test-machine", entities.ImageListOptions{})

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error from engine call with mock connection")
	assert.Nil(t, result, "Should return nil result when engine call fails")
	assert.NotContains(t, err.Error(), "failed to get connection to machine",
		"Error should not be a connection error — connection should have succeeded")
}
