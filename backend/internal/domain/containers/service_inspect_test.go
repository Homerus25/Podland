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

func TestContainerService_Inspect_ReturnsRawReport(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{Name: "/web"}}
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{report}, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "container-123")

	require.NoError(t, err)
	assert.Same(t, report, got)
}

func TestContainerService_Inspect_MissingContainerIsAnError(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		got, err := service.Inspect(context.Background(), "user", "host", "container-123")

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

		got, err := service.Inspect(context.Background(), "user", "host", "container-123")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})
}

func TestContainerService_Inspect_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to inspect container")
}

func TestContainerService_Inspect_ZeroReportsWithPerItemErrorWrapsCause(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{assert.AnError}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to inspect container")
}

func TestContainerService_Inspect_ZeroReportsWithoutErrorsIsAnomaly(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "no inspect report returned for container")
}

// TestContainerService_Inspect_AcquireFailureIsThe502Sentinel pins the
// connection seam: an acquisition failure crosses the service untouched
// (the 502 CONNECTION_FAILED mapping lives in the seam, never here), and
// the engine is never reached.
func TestContainerService_Inspect_AcquireFailureIsThe502Sentinel(t *testing.T) {
	engine := &fakeContainerEngine{
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			panic("engine must not be reached when acquisition fails")
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine, err: appErrors.ErrConnectionFailed})

	got, err := service.Inspect(context.Background(), "user", "host", "container-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}
