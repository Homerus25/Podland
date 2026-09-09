package containers

import (
	"context"
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests for the two logs modes: one-shot (GetLogs
// concatenates stdout+stderr) and follow (GetLogsWithStream converts each
// line to an event via the writers it hands the engine).

func TestContainerService_GetLogs_CombinesStdoutAndStderr(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(_ string, opts entities.ContainerLogsOptions) error {
			writeLogsInto(opts, "out line\n", "err line\n")
			return nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.GetLogs(context.Background(), "user", "machine", "container-123", time.Time{}, 0)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", got.ID)
	assert.Equal(t, "out line\nerr line", got.Logs, "stdout comes first, stderr appended, trailing newline trimmed")
}

func TestContainerService_GetLogs_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.GetLogs(context.Background(), "user", "machine", "container-123", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Nil(t, got)
}

func TestContainerService_GetLogs_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return assert.AnError
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.GetLogs(context.Background(), "user", "machine", "container-123", time.Time{}, 0)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to get container logs")
}

func TestContainerService_GetLogs_MissingParameters(t *testing.T) {
	cases := []struct {
		name        string
		userID      string
		machineID   string
		containerID string
	}{
		{"missing userID", "", "machine", "c-1"},
		{"missing machineID", "user", "", "c-1"},
		{"missing containerID", "user", "machine", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The engine stays unarmed: a refused call must not reach podman.
			service := NewContainerService(fakeEngineProvider{engine: &fakeContainerEngine{}})

			result, err := service.GetLogs(context.Background(), tc.userID, tc.machineID, tc.containerID, time.Time{}, 0)

			assert.Error(t, err, "Should return error for missing parameters")
			assert.Contains(t, err.Error(), "required", "Error should mention required parameter")
			assert.Nil(t, result, "Should return nil result when parameters are missing")
		})
	}
}

func TestContainerService_GetLogsWithStream_EmitsEventsPerLine(t *testing.T) {
	var events []ContainerLogEvent
	engine := &fakeContainerEngine{
		logsFn: func(_ string, opts entities.ContainerLogsOptions) error {
			// Podman prefixes each follow line with its timestamp.
			writeLogsInto(opts,
				"2026-09-01T10:00:00.000000000Z hello\n",
				"2026-09-01T10:00:01.000000000Z broke\n")
			return nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.GetLogsWithStream(context.Background(), "user", "machine", "container-123", time.Time{}, 0,
		func(event ContainerLogEvent) { events = append(events, event) })

	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "INFO", events[0].Level)
	assert.Equal(t, "hello", events[0].Message, "the podman timestamp must be parsed out of the message")
	assert.Equal(t, "2026-09-01T10:00:00Z", events[0].Timestamp)
	assert.Equal(t, "ERROR", events[1].Level, "stderr lines stream at ERROR level")
	assert.Equal(t, "broke", events[1].Message)
}

func TestContainerService_GetLogsWithStream_MapsCancellationToOperationCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return context.Canceled
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.GetLogsWithStream(ctx, "user", "machine", "container-123", time.Time{}, 0,
		func(ContainerLogEvent) {})

	require.ErrorIs(t, err, appErrors.ErrOperationCancelled)
}

func TestContainerService_GetLogsWithStream_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.GetLogsWithStream(context.Background(), "user", "machine", "container-123", time.Time{}, 0,
		func(ContainerLogEvent) {})

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
}
