package images

import (
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

// TestImageService_StubsReturnNotImplemented verifies the stub methods return
// ErrNotImplemented until real implementations land in slices 1–3.
func TestImageService_StubsReturnNotImplemented(t *testing.T) {
	service := NewImageService(testutils.NewMockConnectionManager())

	t.Run("List", func(t *testing.T) {
		result, err := service.List(nil, "user", "machine", entities.ImageListOptions{})
		assert.Nil(t, result)
		assert.Error(t, err)
	})

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
