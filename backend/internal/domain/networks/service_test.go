package networks

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/common/libnetwork/types"
)

// Service behaviour tests run the real NetworkService against the fake
// engine, so the mapping rules below are pinned as behaviour, not
// invocation shape. Connection acquisition (and its 502 mapping) is the
// provider seam: the connections package owns that contract and pins it
// in its own tests.

func TestNetworkService_List_ReturnsEngineNetworks(t *testing.T) {
	networks := []types.Network{
		networkFixture("podman", "bridge", "2026-08-30T10:00:00Z", "10.88.0.0/16"),
		networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16", "fd00:10:89::/64"),
	}
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return networks, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine")

	require.NoError(t, err)
	assert.Equal(t, networks, got)
}

func TestNetworkService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		listFn: func() ([]types.Network, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list networks", "engine failures must be wrapped consistently")
}

// TestNetworkService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestNetworkService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.List(context.Background(), "user", "machine")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
