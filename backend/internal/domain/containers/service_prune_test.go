package containers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

func TestContainerService_Prune_ReturnsEngineReports(t *testing.T) {
	prunes := []*reports.PruneReport{{Id: "stopped-a", Size: 4096}}
	engine := &fakeContainerEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return prunes, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "host")

	require.NoError(t, err)
	assert.Equal(t, prunes, got)
}

func TestContainerService_Prune_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "host")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to prune containers")
}

func TestContainerService_Prune_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewContainerService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.Prune(context.Background(), "user", "host")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
