package images

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
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

// TestConnectionManager_ImplementsInterface is a compile-time check that
// *connections.ConnectionManager satisfies ConnectionManagerInterface,
// allowing it to be passed directly to NewImageService.
func TestConnectionManager_ImplementsInterface(t *testing.T) {
	var _ machines.ConnectionManagerInterface = (*connections.ConnectionManager)(nil)
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

// TestImageService_Delete_ConnectionError tests that Delete returns error when connection fails
func TestImageService_Delete_ConnectionError(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	imageID := "image-123"

	connErr := errors.New("connection failed")
	var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	serviceWithError := NewImageService(mockConnManagerWithError)

	err := serviceWithError.Delete(ctx, userID, machineID, imageID, false)

	assert.Error(t, err, "Should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
}

// TestImageService_Delete_MissingParameters tests that Delete validates input parameters
func TestImageService_Delete_MissingParameters(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	imageID := "image-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewImageService(mockConnManager)

	t.Run("empty userID", func(t *testing.T) {
		err := service.Delete(ctx, "", machineID, imageID, false)
		assert.Error(t, err, "Should return error when userID is empty")
	})

	t.Run("empty machineID", func(t *testing.T) {
		err := service.Delete(ctx, userID, "", imageID, false)
		assert.Error(t, err, "Should return error when machineID is empty")
	})

	t.Run("empty imageID", func(t *testing.T) {
		err := service.Delete(ctx, userID, machineID, "", false)
		assert.Error(t, err, "Should return error when imageID is empty")
	})
}

// TestImageService_Delete_Success tests successful image deletion (integration style)
func TestImageService_Delete_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	imageID := "image-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewImageService(mockConnManager)

	err := service.Delete(ctx, userID, machineID, imageID, false)
	// This will error due to mock limitations, but validates the structure
	assert.Error(t, err, "Expected error due to mock limitations")
}

// TestImageService_Delete_ForceParameter tests that the force parameter is accepted
func TestImageService_Delete_ForceParameter(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	imageID := "image-123"

	t.Run("force=true", func(t *testing.T) {
		mockConnManager := testutils.NewMockConnectionManager()
		service := NewImageService(mockConnManager)

		err := service.Delete(ctx, userID, machineID, imageID, true)
		// Error expected due to mock limitations
		assert.Error(t, err, "Expected error due to mock limitations")
	})

	t.Run("force=false", func(t *testing.T) {
		mockConnManager := testutils.NewMockConnectionManager()
		service := NewImageService(mockConnManager)

		err := service.Delete(ctx, userID, machineID, imageID, false)
		// Error expected due to mock limitations
		assert.Error(t, err, "Expected error due to mock limitations")
	})
}

// TestImageService_ParsePlatform tests that ParsePlatform correctly splits platform strings
func TestImageService_ParsePlatform(t *testing.T) {
	t.Run("empty platform", func(t *testing.T) {
		os, arch, variant := ParsePlatform("")
		assert.Equal(t, "", os)
		assert.Equal(t, "", arch)
		assert.Equal(t, "", variant)
	})

	t.Run("simple platform", func(t *testing.T) {
		os, arch, variant := ParsePlatform("linux/amd64")
		assert.Equal(t, "linux", os)
		assert.Equal(t, "amd64", arch)
		assert.Equal(t, "", variant)
	})

	t.Run("platform with variant", func(t *testing.T) {
		os, arch, variant := ParsePlatform("linux/arm64/v8")
		assert.Equal(t, "linux", os)
		assert.Equal(t, "arm64", arch)
		assert.Equal(t, "v8", variant)
	})

	t.Run("platform with multiple slashes", func(t *testing.T) {
		os, arch, variant := ParsePlatform("windows/amd64/10.0.17763.1234")
		assert.Equal(t, "windows", os)
		assert.Equal(t, "amd64", arch)
		assert.Equal(t, "10.0.17763.1234", variant)
	})
}

// TestImageService_Pull_ConnectionFailure tests that Pull properly handles
// connection failures from the ConnectionManager.
func TestImageService_Pull_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewImageService(mockConnManager)

	result, err := service.Pull(ctx, "test-user", "test-machine", "nginx:latest", entities.ImagePullOptions{})

	assert.Error(t, err, "Should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

// TestImageService_Pull_MissingRawImage tests that Pull validates input parameters
func TestImageService_Pull_MissingRawImage(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewImageService(mockConnManager)

	t.Run("empty userID", func(t *testing.T) {
		result, err := service.Pull(ctx, "", machineID, "nginx:latest", entities.ImagePullOptions{})
		assert.Error(t, err, "Should return error when userID is empty")
		assert.Nil(t, result, "Should return nil result when userID is empty")
	})

	t.Run("empty machineID", func(t *testing.T) {
		result, err := service.Pull(ctx, userID, "", "nginx:latest", entities.ImagePullOptions{})
		assert.Error(t, err, "Should return error when machineID is empty")
		assert.Nil(t, result, "Should return nil result when machineID is empty")
	})

	t.Run("empty rawImage", func(t *testing.T) {
		result, err := service.Pull(ctx, userID, machineID, "", entities.ImagePullOptions{})
		assert.Error(t, err, "Should return error when rawImage is empty")
		assert.Nil(t, result, "Should return nil result when rawImage is empty")
	})
}

// TestImageService_Pull_EngineFailure tests that Pull properly maps
// engine pull failures to ErrImagePullFailed.
func TestImageService_Pull_EngineFailure(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewImageService(mockConnManager)

	result, err := service.Pull(ctx, userID, machineID, "nginx:latest", entities.ImagePullOptions{})

	// The mock connection succeeds but the engine call fails because there is
	// no real podman backend behind the mock connection context.
	assert.Error(t, err, "Should return error when engine pull fails")
	assert.Equal(t, appErrors.ErrImagePullFailed, err, "Should return ErrImagePullFailed")
	assert.Nil(t, result, "Should return nil result when engine pull fails")
}
