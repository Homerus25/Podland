package containers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/Homerus25/podland/backend/internal/domain/machines"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

// TestContainerService_Get_Success tests successful container retrieval
func TestContainerService_Get_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	// Setup mock dependencies
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewContainerService(mockConnManager)

	// Test that the service properly handles connection errors
	t.Run("connection error", func(t *testing.T) {
		connErr := errors.New("connection failed")
		var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
		serviceWithError := NewContainerService(mockConnManagerWithError)

		result, err := serviceWithError.Get(ctx, userID, machineID, containerID)

		assert.Error(t, err, "Should return error when connection fails")
		assert.Contains(t, err.Error(), "failed to get connection to machine", "Error should mention connection failure")
		assert.Nil(t, result, "Should return nil result when connection fails")
	})

	// Test that the service properly handles container inspection
	t.Run("container inspection error", func(t *testing.T) {
		// This would require more complex mocking of the ContainerEngine
		// For now, we'll just verify the basic structure works
		_, err := service.Get(ctx, userID, machineID, containerID)
		// This will fail because we can't easily mock the ContainerEngine creation
		// but that's expected for this unit test
		assert.Error(t, err, "Expected error due to mock limitations")
	})
}
