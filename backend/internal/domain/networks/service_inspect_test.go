package networks

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

func TestNetworkService_Inspect_ReturnsInspectReport(t *testing.T) {
	report := networkInspectFixture("frontend")
	var inspectedName string
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			inspectedName = name
			return []entities.NetworkInspectReport{report}, nil, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "frontend")

	require.NoError(t, err)
	assert.Equal(t, "frontend", inspectedName, "the requested network must reach the host")
	assert.Equal(t, report, got)
}

func TestNetworkService_Inspect_MissingNetworkIsAnError(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such network"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeNetworkEngine{
			inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewNetworkService(fakeEngineProvider{engine: engine})

		got, err := service.Inspect(context.Background(), "user", "host", "ghost-net")

		require.Error(t, err)
		assert.Equal(t, entities.NetworkInspectReport{}, got)
		assert.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	})

	t.Run("per-item refusal", func(t *testing.T) {
		engine := &fakeNetworkEngine{
			inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
				return nil, []error{fmt.Errorf("no such network %q", name)}, nil
			},
		}
		service := NewNetworkService(fakeEngineProvider{engine: engine})

		got, err := service.Inspect(context.Background(), "user", "host", "ghost-net")

		require.Error(t, err)
		assert.Equal(t, entities.NetworkInspectReport{}, got)
		assert.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	})
}

func TestNetworkService_Inspect_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, nil, assert.AnError
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "frontend")

	require.Error(t, err)
	assert.Equal(t, entities.NetworkInspectReport{}, got)
	assert.Contains(t, err.Error(), "failed to inspect network")
}

func TestNetworkService_Inspect_ZeroReportsWithPerItemErrorWrapsCause(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, []error{assert.AnError}, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "frontend")

	require.Error(t, err)
	assert.Equal(t, entities.NetworkInspectReport{}, got)
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, appErrors.ErrNetworkNotFound)
	assert.Contains(t, err.Error(), "failed to inspect network")
}

func TestNetworkService_Inspect_ZeroReportsWithoutErrorsIsAnomaly(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, nil, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "host", "frontend")

	require.Error(t, err)
	assert.Equal(t, entities.NetworkInspectReport{}, got)
	assert.Contains(t, err.Error(), "no inspect report returned for network")
}

// TestNetworkService_Inspect_AcquireFailureIsThe502Sentinel pins the
// service-side half of the connection-failure contract: the provider's
// sentinel crosses untouched — the service never wraps, maps, or
// swallows it (the 502 mapping itself is pinned in the connections
// package).
func TestNetworkService_Inspect_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.Inspect(context.Background(), "user", "host", "frontend")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
