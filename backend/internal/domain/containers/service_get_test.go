package containers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

func TestContainerService_Get_ReturnsInspectReport(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{Name: "web"}}
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{report}, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "container-123")

	require.NoError(t, err)
	assert.Same(t, report, got)
}

func TestContainerService_Get_MissingContainerIsAnError(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "no container found with ID: container-123")
}

func TestContainerService_Get_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to inspect container")
}
