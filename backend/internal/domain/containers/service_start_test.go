package containers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestContainerService_Start_MissingParameters tests that Start validates input parameters
func TestContainerService_Start_MissingParameters(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	// The engine stays unarmed: a refused call must not reach podman.
	service := NewContainerService(fakeEngineProvider{engine: &fakeContainerEngine{}})

	t.Run("empty userID", func(t *testing.T) {
		result, err := service.Start(ctx, "", machineID, containerID)
		assert.Error(t, err, "Should return error when userID is empty")
		assert.Contains(t, err.Error(), "userID is required", "Error should mention userID")
		assert.Nil(t, result, "Should return nil result")
	})

	t.Run("empty machineID", func(t *testing.T) {
		result, err := service.Start(ctx, userID, "", containerID)
		assert.Error(t, err, "Should return error when machineID is empty")
		assert.Contains(t, err.Error(), "machineID is required", "Error should mention machineID")
		assert.Nil(t, result, "Should return nil result")
	})

	t.Run("empty containerID", func(t *testing.T) {
		result, err := service.Start(ctx, userID, machineID, "")
		assert.Error(t, err, "Should return error when containerID is empty")
		assert.Contains(t, err.Error(), "containerID is required", "Error should mention containerID")
		assert.Nil(t, result, "Should return nil result")
	})
}

func TestContainerService_Start_ReportsRunningWithResolvedName(t *testing.T) {
	engine := &fakeContainerEngine{
		startFn: func(string) ([]*entities.ContainerStartReport, error) {
			return []*entities.ContainerStartReport{{Id: "container-123"}}, nil
		},
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			// Podman reports container names with a leading slash.
			return []*entities.ContainerInspectReport{{InspectContainerData: &define.InspectContainerData{Name: "/web"}}}, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Start(context.Background(), "user", "machine", "container-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "web", got.Name, "the leading slash must be trimmed from podman's name")
	assert.Equal(t, "running", got.State)
}

func TestContainerService_Start_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		startFn: func(string) ([]*entities.ContainerStartReport, error) {
			return nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Start(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to start container")
}
