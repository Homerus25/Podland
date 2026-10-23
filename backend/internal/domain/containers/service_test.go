package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
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
	assert.Equal(t, "c-123", got.Id, "the service returns podman's create report untouched")

	require.NotNil(t, captured)
	assert.Equal(t, "nginx:latest", captured.Image)
	assert.Equal(t, "test-container", captured.Name)
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, captured.Command)
	assert.Equal(t, map[string]string{"ENV1": "value1"}, captured.Env)
	assert.Equal(t, map[string]string{"app": "test"}, captured.Labels)
}

// TestContainerService_Create_MapsSecurityOptions pins the security
// mappings at the service seam: a privileged request sets the spec's
// Privileged pointer, the capAdd list reaches the spec verbatim, and a
// request without either leaves the spec unprivileged with no caps.
func TestContainerService_Create_MapsSecurityOptions(t *testing.T) {
	var captured *specgen.SpecGenerator
	engine := &fakeContainerEngine{
		createFn: func(s *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			captured = s
			return &entities.ContainerCreateReport{Id: "c-123"}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	_, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Image:      "nginx:latest",
		Privileged: true,
		CapAdd:     []string{"NET_ADMIN", "SYS_TIME"},
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, captured.Privileged, "a privileged request must set the spec's Privileged pointer")
	assert.True(t, *captured.Privileged)
	assert.Equal(t, []string{"NET_ADMIN", "SYS_TIME"}, captured.CapAdd, "the capAdd list must reach the spec verbatim")
}

// TestContainerService_Create_WithoutSecurityOptionsStaysDefault pins the
// default: a request without privileged or capAdd leaves the spec's
// Privileged pointer unset and CapAdd empty.
func TestContainerService_Create_WithoutSecurityOptionsStaysDefault(t *testing.T) {
	var captured *specgen.SpecGenerator
	engine := &fakeContainerEngine{
		createFn: func(s *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			captured = s
			return &entities.ContainerCreateReport{Id: "c-123"}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	_, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Image: "nginx:latest",
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Nil(t, captured.Privileged, "a request without privileged must leave the spec unprivileged")
	assert.False(t, captured.IsPrivileged())
	assert.Empty(t, captured.CapAdd, "a request without capAdd must leave the spec's CapAdd empty")
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

// TestContainerService_Create_MapsMountFlags pins the bind-mount flag
// mapping at the service seam: selinuxShared, selinuxPrivate, and
// chownUser reach the spec mount's options as z, Z, and U, riding
// alongside the rbind propagation and the requested access mode.
func TestContainerService_Create_MapsMountFlags(t *testing.T) {
	var captured *specgen.SpecGenerator
	engine := &fakeContainerEngine{
		createFn: func(s *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
			captured = s
			return &entities.ContainerCreateReport{Id: "c-123"}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	_, err := service.Create(context.Background(), "user", "machine", ContainerCreateRequest{
		Image: "nginx:latest",
		Mounts: []MountRequest{
			{Source: "/host/ssl", Destination: "/etc/ssl", SelinuxPrivate: true, ChownUser: true},
			{Source: "/host/html", Destination: "/usr/share/nginx/html", Mode: "ro", SelinuxShared: true},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, []runtimespec.Mount{
		{Type: "bind", Source: "/host/ssl", Destination: "/etc/ssl", Options: []string{"rbind", "rw", "Z", "U"}},
		{Type: "bind", Source: "/host/html", Destination: "/usr/share/nginx/html", Options: []string{"rbind", "ro", "z"}},
	}, captured.Mounts, "the mount flags must reach the spec's mount options as z/Z/U alongside rbind and the mode")
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
