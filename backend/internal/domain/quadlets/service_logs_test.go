package quadlets

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

// TestQuadletService_GetLogs_ResolvesSystemdContainerName pins the payoff
// behaviour of the engine seam: for a container-type quadlet without an
// explicit name, logs are fetched from the systemd-spawned container
// (systemd-<basename>) and stdout/stderr combine into one multiline string.
func TestQuadletService_GetLogs_ResolvesSystemdContainerName(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container", Status: "running"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			if opts.StdoutWriter != nil {
				_, _ = opts.StdoutWriter.Write([]byte("out line\n"))
			}
			if opts.StderrWriter != nil {
				_, _ = opts.StderrWriter.Write([]byte("err line\n"))
			}
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "systemd-web", loggedContainer,
		"container-type quadlets must log from the systemd-spawned container")
	assert.Equal(t, "out line\nerr line", got.Logs, "stdout comes first, stderr appended, trailing newline trimmed")
	assert.Equal(t, "web.container", got.ID)
}

// TestQuadletService_GetLogs_ExplicitContainerNameWins pins that an
// explicit ContainerName= key in the unit content overrides the
// systemd-<basename> convention.
func TestQuadletService_GetLogs_ExplicitContainerNameWins(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\nContainerName=my-explicit-container\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	_, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.NoError(t, err)
	assert.Equal(t, "my-explicit-container", loggedContainer)
}

// TestQuadletService_GetLogs_PodQuadletUsesPodContainer pins the pod branch
// of the name resolution: a pod-type quadlet logs from the pod container
// reported by the list.
func TestQuadletService_GetLogs_PodQuadletUsesPodContainer(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "stack.pod", Pod: "stack-the-pod"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Pod]\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	_, err := service.GetLogs(context.Background(), "user", "machine", "stack.pod", time.Time{}, 0)

	require.NoError(t, err)
	assert.Equal(t, "stack-the-pod", loggedContainer)
}

func TestQuadletService_GetLogs_MissingQuadletIs404BeforeLogs(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		// printFn/logsFn stay unarmed: a missing quadlet must not touch
		// the machine beyond the existence check.
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_GetLogs_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\n", nil
		},
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

// TestQuadletService_GetLogsWithStream_EmitsEventsPerLine pins the follow
// mode: each line arrives as an event through the writers the service hands
// the engine, stdout at INFO and stderr at ERROR, with podman's line
// timestamps parsed out.
func TestQuadletService_GetLogsWithStream_EmitsEventsPerLine(t *testing.T) {
	var events []QuadletLogEvent
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\n", nil
		},
		logsFn: func(_ string, opts entities.ContainerLogsOptions) error {
			if opts.StdoutWriter != nil {
				_, _ = opts.StdoutWriter.Write([]byte("2026-09-01T10:00:00.000000000Z hello\n"))
			}
			if opts.StderrWriter != nil {
				_, _ = opts.StderrWriter.Write([]byte("2026-09-01T10:00:01.000000000Z broke\n"))
			}
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	err := service.GetLogsWithStream(context.Background(), "user", "machine", "web.container", time.Time{}, 0,
		func(event QuadletLogEvent) { events = append(events, event) })

	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "INFO", events[0].Level)
	assert.Equal(t, "hello", events[0].Message, "the podman timestamp must be parsed out of the message")
	assert.Equal(t, "2026-09-01T10:00:00Z", events[0].Timestamp)
	assert.Equal(t, "ERROR", events[1].Level)
	assert.Equal(t, "broke", events[1].Message)
}

func TestQuadletService_GetLogsWithStream_MapsCancellationToOperationCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\n", nil
		},
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return context.Canceled
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	err := service.GetLogsWithStream(ctx, "user", "machine", "web.container", time.Time{}, 0,
		func(QuadletLogEvent) {})

	require.ErrorIs(t, err, appErrors.ErrOperationCancelled)
}

// TestQuadletService_GetLogs_ResolveContainerName tests container name resolution
func TestQuadletService_GetLogs_ResolveContainerName(t *testing.T) {
	t.Run("container-type quadlet", func(t *testing.T) {
		quadlet := &entities.ListQuadlet{
			Name: "my-quadlet.container",
			Pod:  "", // No pod, so use the systemd-spawned container name
		}
		containerName := resolveManagedContainerName(quadlet, "my-quadlet.container")
		assert.Equal(t, "systemd-my-quadlet", containerName,
			"Container-type quadlet must resolve to the systemd-spawned container (podman names it systemd-<basename>)")
	})

	t.Run("pod-type quadlet with pod container", func(t *testing.T) {
		quadlet := &entities.ListQuadlet{
			Name: "my-pod",
			Pod:  "my-pod-container", // Use the pod container name
		}
		containerName := resolveManagedContainerName(quadlet, "my-pod")
		assert.Equal(t, "my-pod-container", containerName, "Pod-type quadlet should use pod container name")
	})

	t.Run("nil quadlet", func(t *testing.T) {
		containerName := resolveManagedContainerName(nil, "fallback-name")
		assert.Equal(t, "fallback-name", containerName, "Nil quadlet should fall back to provided name")
	})
}

// TestQuadletService_GetLogs_ExplicitContainerNameFromContent tests the
// ContainerName= extraction that overrides the systemd-<basename> convention.
func TestQuadletService_GetLogs_ExplicitContainerNameFromContent(t *testing.T) {
	t.Run("explicit ContainerName wins", func(t *testing.T) {
		content := `[Container]
Image=busybox
Exec=sh -c "while true; do echo hi; sleep 1; done"
ContainerName=my-explicit-container
`
		assert.Equal(t, "my-explicit-container", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("absent key returns empty", func(t *testing.T) {
		content := `[Container]
Image=busybox
Exec=sleep 3600
`
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("case-insensitive key, trimmed and unquoted value", func(t *testing.T) {
		content := "[Container]\ncontainername=  \"quoted-name\"  \n"
		assert.Equal(t, "quoted-name", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("comments and lookalike keys are ignored", func(t *testing.T) {
		content := `[Container]
# ContainerName=commented-out
Label=this.looks=like=an=assignment
Exec=echo ContainerName=not-a-key
`
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("empty value returns empty", func(t *testing.T) {
		content := "[Container]\nContainerName=\n"
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})
}

// TestQuadletLogEventWriter tests the log event writer
func TestQuadletLogEventWriter(t *testing.T) {
	t.Run("basic write with timestamp", func(t *testing.T) {
		var capturedEvent QuadletLogEvent
		writer := &quadletLogEventWriter{
			callback:   func(event QuadletLogEvent) { capturedEvent = event },
			level:      "INFO",
			timestamps: true,
		}

		testData := []byte("2024-01-01T00:00:00.000000000Z Test log message\n")
		n, err := writer.Write(testData)

		require.NoError(t, err, "Write should not return error")
		assert.Equal(t, len(testData), n, "Write should return correct byte count")
		assert.Contains(t, capturedEvent.Timestamp, "2024-01-01", "Should parse timestamp date correctly")
		assert.Equal(t, "INFO", capturedEvent.Level, "Should use provided level")
		assert.Equal(t, "Test log message", capturedEvent.Message, "Should extract message correctly")
	})

	t.Run("write without timestamp", func(t *testing.T) {
		var capturedEvent QuadletLogEvent
		writer := &quadletLogEventWriter{
			callback:   func(event QuadletLogEvent) { capturedEvent = event },
			level:      "ERROR",
			timestamps: false,
		}

		testData := []byte("Test log message without timestamp\n")
		n, err := writer.Write(testData)

		require.NoError(t, err, "Write should not return error")
		assert.Equal(t, len(testData), n, "Write should return correct byte count")
		assert.NotEmpty(t, capturedEvent.Timestamp, "Should generate timestamp when not parsing")
		assert.Equal(t, "ERROR", capturedEvent.Level, "Should use provided level")
		assert.Equal(t, "Test log message without timestamp", capturedEvent.Message, "Should use full message as message")
	})

	t.Run("empty line", func(t *testing.T) {
		var capturedEvent QuadletLogEvent
		writer := &quadletLogEventWriter{
			callback:   func(event QuadletLogEvent) { capturedEvent = event },
			level:      "INFO",
			timestamps: true,
		}

		testData := []byte("\n")
		n, err := writer.Write(testData)

		require.NoError(t, err, "Write should not return error")
		assert.Equal(t, len(testData), n, "Write should return correct byte count")
		assert.Empty(t, capturedEvent.Message, "Should not emit event for empty line")
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		var capturedEvent QuadletLogEvent
		writer := &quadletLogEventWriter{
			callback:   func(event QuadletLogEvent) { capturedEvent = event },
			level:      "WARN",
			timestamps: true,
		}

		testData := []byte("invalid-timestamp Test log message\n")
		n, err := writer.Write(testData)

		require.NoError(t, err, "Write should not return error")
		assert.Equal(t, len(testData), n, "Write should return correct byte count")
		assert.NotEmpty(t, capturedEvent.Timestamp, "Should generate timestamp when parsing fails")
		assert.Equal(t, "WARN", capturedEvent.Level, "Should use provided level")
		assert.Equal(t, "invalid-timestamp Test log message", capturedEvent.Message, "Should use full message as message when parsing fails")
	})

	t.Run("multiple events", func(t *testing.T) {
		var capturedEvents []QuadletLogEvent
		writer := &quadletLogEventWriter{
			callback: func(event QuadletLogEvent) {
				capturedEvents = append(capturedEvents, event)
			},
			level:      "INFO",
			timestamps: true,
		}

		testData1 := []byte("2024-01-01T00:00:00.000000000Z First message\n")
		testData2 := []byte("2024-01-01T00:00:01.000000000Z Second message\n")

		_, err := writer.Write(testData1)
		require.NoError(t, err)

		_, err = writer.Write(testData2)
		require.NoError(t, err)

		assert.Len(t, capturedEvents, 2, "Should capture two events")
		assert.Equal(t, "First message", capturedEvents[0].Message, "First event should have first message")
		assert.Equal(t, "Second message", capturedEvents[1].Message, "Second event should have second message")
	})
}
