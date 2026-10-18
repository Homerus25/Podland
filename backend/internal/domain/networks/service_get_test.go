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

func TestNetworkService_Get_ReturnsMappedDetail(t *testing.T) {
	report := networkInspectFixture("frontend")
	var inspectedName string
	engine := &fakeNetworkEngine{
		inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
			inspectedName = name
			return []entities.NetworkInspectReport{report}, nil, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "frontend")

	require.NoError(t, err)
	assert.Equal(t, "frontend", inspectedName, "the requested network must reach the machine")
	assert.Equal(t, NetworkDetailResponse{
		Name:      "frontend",
		ID:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		Driver:    "bridge",
		CreatedAt: "2026-09-01T12:30:00Z",
		Subnets:   []string{},
		Containers: []NetworkContainerRef{
			{ID: "ctr-a", Name: "web"},
		},
	}, got)
}

func TestNetworkService_Get_MissingNetworkIsAnError(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such network"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeNetworkEngine{
			inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewNetworkService(fakeEngineProvider{engine: engine})

		got, err := service.Get(context.Background(), "user", "machine", "ghost-net")

		require.Error(t, err)
		assert.Equal(t, NetworkDetailResponse{}, got)
		assert.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	})

	t.Run("per-item refusal", func(t *testing.T) {
		engine := &fakeNetworkEngine{
			inspectFn: func(name string) ([]entities.NetworkInspectReport, []error, error) {
				return nil, []error{fmt.Errorf("no such network %q", name)}, nil
			},
		}
		service := NewNetworkService(fakeEngineProvider{engine: engine})

		got, err := service.Get(context.Background(), "user", "machine", "ghost-net")

		require.Error(t, err)
		assert.Equal(t, NetworkDetailResponse{}, got)
		assert.ErrorIs(t, err, appErrors.ErrNetworkNotFound)
	})
}

func TestNetworkService_Get_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		inspectFn: func(string) ([]entities.NetworkInspectReport, []error, error) {
			return nil, nil, assert.AnError
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "frontend")

	require.Error(t, err)
	assert.Equal(t, NetworkDetailResponse{}, got)
	assert.Contains(t, err.Error(), "failed to inspect network")
}

// TestNetworkService_Get_AcquireFailureIsThe502Sentinel pins the
// service-side half of the connection-failure contract: the provider's
// sentinel crosses untouched — the service never wraps, maps, or
// swallows it (the 502 mapping itself is pinned in the connections
// package).
func TestNetworkService_Get_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.Get(context.Background(), "user", "machine", "frontend")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
