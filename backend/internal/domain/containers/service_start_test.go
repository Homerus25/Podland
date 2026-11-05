package containers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// stoppedInspect is the pre-action inspect answer for a container that
// may be started: it exists and is not running.
func stoppedInspect(string) ([]*entities.ContainerInspectReport, []error, error) {
	return []*entities.ContainerInspectReport{{
		InspectContainerData: &define.InspectContainerData{
			Name:  "/web",
			State: &define.InspectContainerState{Status: "exited", Running: false},
		},
	}}, nil, nil
}

func TestContainerService_Start_ResolvesContainerName(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: stoppedInspect,
		startFn:   func(string) error { return nil },
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Start(context.Background(), "user", "host", "container-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "web", got.Name, "the leading slash must be trimmed from podman's name")
}

// TestContainerService_Start_AlreadyRunningAnswers400 pins the
// documented 400 terminal: starting an already-running container
// answers ErrContainerAlreadyRunning before any start call — podman's
// refusal produces no error shape through the tunnel (verified live,
// issue #270), so the precondition is state-based. A paused container
// counts as running: podman's State.Running is true for it.
func TestContainerService_Start_AlreadyRunningAnswers400(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return []*entities.ContainerInspectReport{{
					InspectContainerData: &define.InspectContainerData{
						Name:  "/web",
						State: &define.InspectContainerState{Status: "running", Running: true},
					},
				}}, nil, nil
			},
			// startFn stays unarmed: the precondition must answer
			// before the action call, and an unexpected call panics.
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Start(context.Background(), "user", "host", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerAlreadyRunning)
		assert.Nil(t, got)
	})

	t.Run("paused counts as running", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return []*entities.ContainerInspectReport{{
					InspectContainerData: &define.InspectContainerData{
						Name:  "/web",
						State: &define.InspectContainerState{Status: "paused", Running: true, Paused: true},
					},
				}}, nil, nil
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Start(context.Background(), "user", "host", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerAlreadyRunning)
		assert.Nil(t, got)
	})
}

func TestContainerService_Start_MapsNotFoundTo404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("inspect transport-level refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Start(context.Background(), "user", "host", "container-123")

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

		got, err := service.Start(context.Background(), "user", "host", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
		assert.Nil(t, got)
	})

	t.Run("start refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: stoppedInspect,
			startFn:   func(string) error { return notFound },
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Start(context.Background(), "user", "host", "container-123")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
		assert.Nil(t, got)
	})
}

func TestContainerService_Start_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: stoppedInspect,
		startFn:   func(string) error { return assert.AnError },
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Start(context.Background(), "user", "host", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to start container")
}
