package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests for the one-shot stats snapshot: the forwarded
// engine options (the target IDs and the all flag) and the not-found
// classification carrying podman's own message.

func TestContainerService_Stats_ForwardsAllOptions(t *testing.T) {
	var gotIDs []string
	var gotAll bool
	engine := &fakeContainerEngine{
		statsFn: func(ids []string, all bool) ([]define.ContainerStats, error) {
			gotIDs, gotAll = ids, all
			return []define.ContainerStats{{ContainerID: "running-a", Name: "web"}}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Stats(context.Background(), "user", "machine", nil, true)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "running-a", got[0].ContainerID)
	assert.Nil(t, gotIDs, "the all-stats call passes no target IDs")
	assert.True(t, gotAll, "the all-stats call must set the all flag")
}

func TestContainerService_Stats_ForwardsTargetOptions(t *testing.T) {
	var gotIDs []string
	var gotAll bool
	engine := &fakeContainerEngine{
		statsFn: func(ids []string, all bool) ([]define.ContainerStats, error) {
			gotIDs, gotAll = ids, all
			return []define.ContainerStats{{ContainerID: "container-123", Name: "web"}}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Stats(context.Background(), "user", "machine", []string{"container-123"}, false)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "container-123", got[0].ContainerID)
	assert.Equal(t, []string{"container-123"}, gotIDs, "the per-container call must forward the target ID")
	assert.False(t, gotAll, "the per-container call must not set the all flag")
}

// TestContainerService_Stats_MapsMissingContainerTo404 pins the not-found
// classification: podman's stats handler maps any first-report error to
// 404 — an unknown container and a known-but-not-running one alike — so
// the domain answers its existing not-found sentinel with podman's own
// message riding along as the only disambiguator.
func TestContainerService_Stats_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Stats(context.Background(), "user", "machine", []string{"container-123"}, false)

	// WithMessage copies the sentinel (AppError has no Is), so the
	// classification asserts on the AppError's code, not the chain.
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "a stats call for a missing container must answer CONTAINER_NOT_FOUND")
	assert.Equal(t, "CONTAINER_NOT_FOUND", appErr.Code)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
	assert.Contains(t, appErr.Message, "no such container", "podman's own message must ride along as the disambiguator")
	assert.Nil(t, got)
}

// TestContainerService_Stats_WrapsEngineFailure pins the non-not-found
// branch: any other engine failure is wrapped, never answered as a
// stats success.
func TestContainerService_Stats_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		statsFn: func([]string, bool) ([]define.ContainerStats, error) {
			return nil, assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Stats(context.Background(), "user", "machine", nil, true)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to get container stats")
}

// TestContainerService_Stats_ConnectionFailureAnswers502 pins the
// connection-seam crossing: an acquisition failure surfaces the 502
// sentinel untouched, exactly like every other containers operation.
func TestContainerService_Stats_ConnectionFailureAnswers502(t *testing.T) {
	service := NewContainerService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.Stats(context.Background(), "user", "machine", nil, true)

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
}
