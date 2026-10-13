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
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Get(context.Background(), "user", "machine", "container-123")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})

	t.Run("per-item refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, []error{fmt.Errorf("no such container %q", id)}, nil
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Get(context.Background(), "user", "machine", "container-123")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})
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

func TestContainerService_Get_ZeroReportsWithPerItemErrorWrapsCause(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{assert.AnError}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to inspect container")
}

func TestContainerService_Get_ZeroReportsWithoutErrorsIsAnomaly(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "no inspect report returned for container")
}
