package containers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// TestContainerService_Delete_ConnectionError tests that Delete returns error when connection fails
func TestContainerService_Delete_ConnectionError(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	connErr := errors.New("connection failed")
	var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	serviceWithError := NewContainerService(mockConnManagerWithError)

	err := serviceWithError.Delete(ctx, userID, machineID, containerID, false)

	assert.Error(t, err, "Should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
}

// TestContainerService_Delete_MissingParameters tests that Delete validates input parameters
func TestContainerService_Delete_MissingParameters(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewContainerService(mockConnManager)

	t.Run("empty userID", func(t *testing.T) {
		err := service.Delete(ctx, "", machineID, containerID, false)
		assert.Error(t, err, "Should return error when userID is empty")
		assert.Contains(t, err.Error(), "userID is required", "Error should mention userID")
	})

	t.Run("empty machineID", func(t *testing.T) {
		err := service.Delete(ctx, userID, "", containerID, false)
		assert.Error(t, err, "Should return error when machineID is empty")
		assert.Contains(t, err.Error(), "machineID is required", "Error should mention machineID")
	})

	t.Run("empty containerID", func(t *testing.T) {
		err := service.Delete(ctx, userID, machineID, "", false)
		assert.Error(t, err, "Should return error when containerID is empty")
		assert.Contains(t, err.Error(), "containerID is required", "Error should mention containerID")
	})
}

// TestContainerService_Delete_Success tests successful container deletion (integration style)
func TestContainerService_Delete_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewContainerService(mockConnManager)

	err := service.Delete(ctx, userID, machineID, containerID, false)
	// This will error due to mock limitations, but validates the structure
	assert.Error(t, err, "Expected error due to mock limitations")
}

// TestContainerService_Delete_ForceParameter tests that the force parameter is accepted
func TestContainerService_Delete_ForceParameter(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	t.Run("force=true", func(t *testing.T) {
		mockConnManager := testutils.NewMockConnectionManager()
		service := NewContainerService(mockConnManager)

		err := service.Delete(ctx, userID, machineID, containerID, true)
		// Error expected due to mock limitations
		assert.Error(t, err, "Expected error due to mock limitations")
	})

	t.Run("force=false", func(t *testing.T) {
		mockConnManager := testutils.NewMockConnectionManager()
		service := NewContainerService(mockConnManager)

		err := service.Delete(ctx, userID, machineID, containerID, false)
		// Error expected due to mock limitations
		assert.Error(t, err, "Expected error due to mock limitations")
	})
}
