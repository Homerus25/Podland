package networks

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// Prune service behaviour, mirroring the volumes/pods prune reference:
// podman's per-network reports pass through untouched, engine failures
// wrap with domain context, and the acquisition sentinel crosses
// untouched.

func TestNetworkService_Prune_ReturnsEngineReports(t *testing.T) {
	prunes := []*entities.NetworkPruneReport{
		{Name: "unused-a"},
		{Name: "unused-b", Error: assert.AnError},
	}
	engine := &fakeNetworkEngine{
		pruneFn: func() ([]*entities.NetworkPruneReport, error) {
			return prunes, nil
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "machine")

	require.NoError(t, err)
	assert.Equal(t, prunes, got, "the engine's reports must pass through untouched")
}

func TestNetworkService_Prune_WrapsEngineFailure(t *testing.T) {
	engine := &fakeNetworkEngine{
		pruneFn: func() ([]*entities.NetworkPruneReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewNetworkService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "machine")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to prune networks", "engine failures must be wrapped consistently")
}

func TestNetworkService_Prune_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewNetworkService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.Prune(context.Background(), "user", "machine")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
