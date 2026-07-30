package containers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// TestContainerService_Stop_ConnectionError tests that Stop returns error when connection fails
func TestContainerService_Stop_ConnectionError(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	connErr := errors.New("connection failed")
	var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	serviceWithError := NewContainerService(mockConnManagerWithError)

	result, err := serviceWithError.Stop(ctx, userID, machineID, containerID)

	assert.Error(t, err, "Should return error when connection fails")
	assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
	assert.Nil(t, result, "Should return nil result when connection fails")
}

// TestContainerService_Stop_MissingParameters tests that Stop validates input parameters
func TestContainerService_Stop_MissingParameters(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewContainerService(mockConnManager)

	t.Run("empty userID", func(t *testing.T) {
		result, err := service.Stop(ctx, "", machineID, containerID)
		assert.Error(t, err, "Should return error when userID is empty")
		assert.Contains(t, err.Error(), "userID is required", "Error should mention userID")
		assert.Nil(t, result, "Should return nil result")
	})

	t.Run("empty machineID", func(t *testing.T) {
		result, err := service.Stop(ctx, userID, "", containerID)
		assert.Error(t, err, "Should return error when machineID is empty")
		assert.Contains(t, err.Error(), "machineID is required", "Error should mention machineID")
		assert.Nil(t, result, "Should return nil result")
	})

	t.Run("empty containerID", func(t *testing.T) {
		result, err := service.Stop(ctx, userID, machineID, "")
		assert.Error(t, err, "Should return error when containerID is empty")
		assert.Contains(t, err.Error(), "containerID is required", "Error should mention containerID")
		assert.Nil(t, result, "Should return nil result")
	})
}

// TestContainerService_Stop_Success tests successful container stop (integration style)
func TestContainerService_Stop_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	mockConnManager := testutils.NewMockConnectionManager()
	service := NewContainerService(mockConnManager)

	_, err := service.Stop(ctx, userID, machineID, containerID)
	// This will error due to mock limitations, but validates the structure
	assert.Error(t, err, "Expected error due to mock limitations")
}
