package containers

import (
	"context"
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests for the two logs modes: one-shot (the kit's
// FetchLogs owns the buffered ritual, so only the domain-owned mapping
// and validation are pinned here) and follow (the kit's FollowLogs owns
// the follow ritual, so only validation, dispatch, and the promptness of
// the cancellation release are pinned here).

func TestContainerService_GetLogs_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.GetLogs(context.Background(), "user", "host", "container-123", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Nil(t, got)
}

func TestContainerService_GetLogsWithStream_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.GetLogsWithStream(context.Background(), "user", "host", "container-123", time.Time{}, 0,
		func(streaming.LogEvent) {})

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
}

// TestContainerService_GetLogsWithStream_ReturnsPromptlyOnCancel pins
// the release regression: a blocking engine Logs call plus a cancelled
// context must return the operation-cancelled sentinel promptly (the
// shared promptness harness owns the ritual).
func TestContainerService_GetLogsWithStream_ReturnsPromptlyOnCancel(t *testing.T) {
	started, block := testutils.CancelStubs(t)
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			close(started)
			<-block
			return nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	testutils.RequirePromptReturnOnCancel(t, "GetLogsWithStream", started,
		func(ctx context.Context) error {
			return service.GetLogsWithStream(ctx, "user", "host", "container-123", time.Time{}, 0,
				func(streaming.LogEvent) {})
		})
}
