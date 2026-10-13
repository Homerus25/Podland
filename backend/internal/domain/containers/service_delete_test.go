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
