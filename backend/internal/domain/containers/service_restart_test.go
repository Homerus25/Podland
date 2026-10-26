package containers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

func TestContainerService_Restart_ResolvesContainerName(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: runningInspect,
		restartFn: func(string) error { return nil },
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Restart(context.Background(), "user", "machine", "container-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "web", got.Name, "the leading slash must be trimmed from podman's name")
}

// TestContainerService_Restart_NotRunningAnswers400 pins the documented
// 400 terminal: restarting a container that is not running answers
// ErrContainerNotRunning before any restart call, mirroring Stop's
// state-based precondition — a stopped container is Start's caller, not
// this one.
func TestContainerService_Restart_NotRunningAnswers400(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: stoppedInspect,
		// restartFn stays unarmed: the precondition must answer
		// before the action call, and an unexpected call panics.
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Restart(context.Background(), "user", "machine", "container-123")

	require.ErrorIs(t, err, appErrors.ErrContainerNotRunning)
	assert.Nil(t, got)
}

func TestContainerService_Restart_MapsNotFoundTo404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("inspect transport-level refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Restart(context.Background(), "user", "machine", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
		assert.Nil(t, got)
	})

	t.Run("inspect per-item refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				// The tunnel's not-found answer for inspect arrives
				// as a per-item error with zero reports.
				return nil, []error{fmt.Errorf("no such container %q", "container-123")}, nil
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Restart(context.Background(), "user", "machine", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
		assert.Nil(t, got)
	})

	t.Run("restart refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: runningInspect,
			restartFn: func(string) error { return notFound },
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Restart(context.Background(), "user", "machine", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
		assert.Nil(t, got)
	})
}

func TestContainerService_Restart_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: runningInspect,
		restartFn: func(string) error { return assert.AnError },
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Restart(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to restart container")
}
