package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
)

// Service behaviour tests run the real ContainerService against the fake
// engine. Connection acquisition (and its 502 mapping) is the provider
// seam: the connections package owns that contract and pins it in its own
// tests.

func TestContainerService_List_ReturnsEngineContainers(t *testing.T) {
	containers := []entities.ListContainer{
		{ID: "container-1", Names: []string{"test-container"}, Image: "test-image", State: "running"},
	}
	engine := &fakeContainerEngine{
		listFn: func(entities.ContainerListOptions) ([]entities.ListContainer, error) {
			return containers, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.ContainerListOptions{All: true})

	require.NoError(t, err)
	assert.Equal(t, containers, got)
}

func TestContainerService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		listFn: func(entities.ContainerListOptions) ([]entities.ListContainer, error) {
			return nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.ContainerListOptions{})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list containers", "engine failures must be wrapped consistently")
}

func TestContainerService_Create_BuildsSpecFromRequest(t *testing.T) {
	var captured *specgen.SpecGenerator
	engine := &fakeContainerEngine{
		createFn: func(s *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			captured = s
			return &entities.ContainerCreateReport{Id: "c-123"}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Name:        "test-container",
		Image:       "nginx:latest",
		Command:     []string{"/bin/sh", "-c", "echo hello"},
		Environment: map[string]string{"ENV1": "value1"},
		Labels:      map[string]string{"app": "test"},
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "c-123", got.ID)
	assert.Equal(t, "test-container", got.Name)
	assert.Equal(t, "created", got.State)
	assert.Equal(t, "nginx:latest", got.Image)

	require.NotNil(t, captured)
	assert.Equal(t, "nginx:latest", captured.Image)
	assert.Equal(t, "test-container", captured.Name)
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, captured.Command)
	assert.Equal(t, map[string]string{"ENV1": "value1"}, captured.Env)
	assert.Equal(t, map[string]string{"app": "test"}, captured.Labels)
}

func TestContainerService_Create_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine",
		ContainerCreateRequest{Name: "c", Image: "nginx:latest"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to create container")
}

// TestContainerService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestContainerService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewContainerService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.List(context.Background(), "user", "machine", entities.ContainerListOptions{})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
