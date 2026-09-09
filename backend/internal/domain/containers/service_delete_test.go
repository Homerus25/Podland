package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestContainerService_Delete_MissingParameters tests that Delete validates input parameters
func TestContainerService_Delete_MissingParameters(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	containerID := "container-123"

	// The engine stays unarmed: a refused call must not reach podman.
	service := NewContainerService(fakeEngineProvider{engine: &fakeContainerEngine{}})

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

func TestContainerService_Delete_Succeeds(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) ([]*reports.RmReport, error) {
			gotForce = force
			return []*reports.RmReport{{Id: id}}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "machine", "container-123", true)

	require.NoError(t, err)
	assert.True(t, gotForce, "the force flag must reach the engine")
}

func TestContainerService_Delete_MapsNotFoundTo404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			removeFn: func(string, bool) ([]*reports.RmReport, error) {
				return nil, notFound
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "container-123", false)

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})

	t.Run("per-report refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			removeFn: func(string, bool) ([]*reports.RmReport, error) {
				return []*reports.RmReport{{Err: notFound}}, nil
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "container-123", false)

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})
}

func TestContainerService_Delete_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) ([]*reports.RmReport, error) {
			return nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "machine", "container-123", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove container")
}
