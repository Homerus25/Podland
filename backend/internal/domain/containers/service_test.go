package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/errorhandling"
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

// TestContainerService_Create_ReturnsEngineReport pins the thin lease's
// success half: the service hands the built spec to the engine and
// returns podman's create report untouched. The request → specgen
// mapping itself is pinned as direct builder table tests
// (create_spec_test.go).
func TestContainerService_Create_ReturnsEngineReport(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return &entities.ContainerCreateReport{Id: "c-123"}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Name:  "test-container",
		Image: "nginx:latest",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "c-123", got.Id, "the service returns podman's create report untouched")
}

// TestContainerService_Create_MissingPodIsPodNotFound pins the not-found
// classification when a pod was requested: podman's not-found answer maps
// to the 404 POD_NOT_FOUND sentinel, not IMAGE_NOT_FOUND.
func TestContainerService_Create_MissingPodIsPodNotFound(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such pod"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Image: "nginx:latest",
		Pod:   "missing-pod",
	})

	// WithMessage copies the sentinel (the pods domain's precedent), so
	// the classification asserts on the AppError's code, not the chain.
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "a create whose pod is missing must answer POD_NOT_FOUND")
	assert.Equal(t, "POD_NOT_FOUND", appErr.Code)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Contains(t, appErr.Message, "no such pod", "the message must carry podman's not-found text")
	assert.Nil(t, got)
}

// TestContainerService_Create_MissingImageWithoutPodStaysImageNotFound
// pins the not-found classification when no pod was requested: podman's
// not-found answer keeps mapping to the 404 IMAGE_NOT_FOUND sentinel.
func TestContainerService_Create_MissingImageWithoutPodStaysImageNotFound(t *testing.T) {
	engine := &fakeContainerEngine{
		createFn: func(*specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Image: "missing:latest",
	})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "a create without a pod keeps the IMAGE_NOT_FOUND mapping")
	assert.Equal(t, "IMAGE_NOT_FOUND", appErr.Code)
	assert.Contains(t, appErr.Message, "no such image", "the message must carry podman's not-found text")
	assert.Nil(t, got)
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
