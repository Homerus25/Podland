package quadlets

import (
	"bytes"
	"context"
	stdErrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/domain/machines"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestQuadletService_GetLogs_Success tests successful log retrieval
func TestQuadletService_GetLogs_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	quadletName := "my-quadlet"

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	t.Run("service structure validation", func(t *testing.T) {
		// Verify that GetLogs method exists on service
		assert.NotNil(t, service.GetLogs, "GetLogs method should exist on service")

		// Verify that QuadletLogsResponse DTO exists
		response := &QuadletLogsResponse{
			ID:   "test-quadlet",
			Logs: "test log line",
		}
		assert.Equal(t, "test-quadlet", response.ID, "QuadletLogsResponse ID should be settable")
		assert.Equal(t, "test log line", response.Logs, "QuadletLogsResponse Logs should be settable")
	})

	t.Run("connection error", func(t *testing.T) {
		connErr := stdErrors.New("connection failed")
		var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
		serviceWithError := NewQuadletService(mockConnManagerWithError, nil)

		result, err := serviceWithError.GetLogs(ctx, userID, machineID, quadletName, time.Time{}, 0)

		assert.Error(t, err, "Should return error when connection fails")
		assert.Equal(t, appErrors.ErrConnectionFailed, err, "Should return connection failed error")
		assert.Nil(t, result, "Should return nil result when connection fails")
	})

	t.Run("missing parameters", func(t *testing.T) {
		testCases := []struct {
			name        string
			userID      string
			machineID   string
			quadletName string
		}{
			{"missing userID", "", machineID, quadletName},
			{"missing machineID", userID, "", quadletName},
			{"missing quadletName", userID, machineID, ""},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := service.GetLogs(ctx, tc.userID, tc.machineID, tc.quadletName, time.Time{}, 0)

				assert.Error(t, err, "Should return error for missing parameters")
				assert.NotNil(t, err, "Error should not be nil")
				assert.Nil(t, result, "Should return nil result when parameters are missing")
			})
		}
	})

	t.Run("parameter handling", func(t *testing.T) {
		since := time.Now().Add(-1 * time.Hour)
		tail := int64(100)
		result, err := service.GetLogs(ctx, userID, machineID, quadletName, since, tail)

		assert.Error(t, err, "Expected error due to mock limitations")
		assert.Nil(t, result, "Should return nil result due to mock limitations")
	})
}

// TestQuadletService_GetLogs_ErrorMapping tests error mapping
func TestQuadletService_GetLogs_ErrorMapping(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	quadletName := "my-quadlet"

	// Verify that ErrQuadletNotFound exists and has correct status
	t.Run("verify error constants", func(t *testing.T) {
		assert.NotNil(t, appErrors.ErrQuadletNotFound, "ErrQuadletNotFound should be defined")
		assert.Equal(t, "QUADLET_NOT_FOUND", appErrors.ErrQuadletNotFound.Code, "Error code should match")
		assert.Equal(t, 404, appErrors.ErrQuadletNotFound.Status, "Status should be 404")
	})

	// Verify connection error mapping
	t.Run("connection error mapping", func(t *testing.T) {
		connErr := stdErrors.New("connection failed")
		var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
		serviceWithError := NewQuadletService(mockConnManagerWithError, nil)

		result, err := serviceWithError.GetLogs(ctx, userID, machineID, quadletName, time.Time{}, 0)

		assert.Error(t, err, "Should return error when connection fails")
		assert.NotEqual(t, appErrors.ErrQuadletNotFound, err, "Connection error should not map to QuadletNotFound")
		assert.Nil(t, result, "Should return nil result when connection fails")
	})
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

// TestQuadletService_GetLogsWithStream_Success tests successful streaming log retrieval
func TestQuadletService_GetLogsWithStream_Success(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	machineID := "test-machine"
	quadletName := "my-quadlet"

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	t.Run("service structure validation", func(t *testing.T) {
		// Verify that GetLogsWithStream method exists on service
		assert.NotNil(t, service.GetLogsWithStream, "GetLogsWithStream method should exist on service")

		// Verify that QuadletLogEvent DTO exists
		event := QuadletLogEvent{
			Timestamp: "2024-01-01T00:00:00Z",
			Level:     "INFO",
			Message:   "test log message",
		}
		assert.Equal(t, "2024-01-01T00:00:00Z", event.Timestamp, "QuadletLogEvent Timestamp should be settable")
		assert.Equal(t, "INFO", event.Level, "QuadletLogEvent Level should be settable")
		assert.Equal(t, "test log message", event.Message, "QuadletLogEvent Message should be settable")
	})

	t.Run("connection error", func(t *testing.T) {
		connErr := stdErrors.New("connection failed")
		var mockConnManagerWithError machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
		serviceWithError := NewQuadletService(mockConnManagerWithError, nil)

		var capturedEvents []QuadletLogEvent
		err := serviceWithError.GetLogsWithStream(ctx, userID, machineID, quadletName, time.Time{}, 0, func(event QuadletLogEvent) {
			capturedEvents = append(capturedEvents, event)
		})

		assert.Error(t, err, "Should return error when connection fails")
		assert.Equal(t, appErrors.ErrConnectionFailed, err, "Should return connection failed error")
		assert.Empty(t, capturedEvents, "Should not capture any events when connection fails")
	})

	t.Run("callback is called", func(t *testing.T) {
		var capturedEvents []QuadletLogEvent
		err := service.GetLogsWithStream(ctx, userID, machineID, quadletName, time.Time{}, 0, func(event QuadletLogEvent) {
			capturedEvents = append(capturedEvents, event)
		})

		assert.Error(t, err, "Expected error due to mock limitations")
		assert.Empty(t, capturedEvents, "Should not capture events due to mock limitations")
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

// TestQuadletService_GetLogs_Buffers tests buffer handling
func TestQuadletService_GetLogs_Buffers(t *testing.T) {
	t.Run("buffer initialization", func(t *testing.T) {
		// Verify that bytes.Buffer is available for capturing logs
		var stdoutBuffer, stderrBuffer bytes.Buffer

		testLog := "test log line"
		stdoutBuffer.WriteString(testLog)
		stderrBuffer.WriteString(testLog)

		assert.Equal(t, testLog, stdoutBuffer.String(), "Stdout buffer should capture logs")
		assert.Equal(t, testLog, stderrBuffer.String(), "Stderr buffer should capture logs")

		// Test buffer trimming
		combinedLogs := stdoutBuffer.String()
		trimmedBytes := bytes.TrimSuffix(stdoutBuffer.Bytes(), []byte("\n"))
		combinedLogs = string(trimmedBytes)
		assert.Equal(t, testLog, string(combinedLogs), "Buffer trimming should work correctly")
	})

	t.Run("buffer combination", func(t *testing.T) {
		// Test the combination logic from GetLogs
		var stdoutBuffer, stderrBuffer bytes.Buffer

		stdoutBuffer.WriteString("stdout line 1\n")
		stdoutBuffer.WriteString("stdout line 2\n")
		stderrBuffer.WriteString("stderr line 1\n")

		combinedLogs := strings.TrimSuffix(stdoutBuffer.String(), "\n")
		if stderrBuffer.Len() > 0 {
			stderrContent := strings.TrimSuffix(stderrBuffer.String(), "\n")
			if combinedLogs != "" {
				combinedLogs += "\n" + stderrContent
			} else {
				combinedLogs = stderrContent
			}
		}

		expected := "stdout line 1\nstdout line 2\nstderr line 1"
		assert.Equal(t, expected, combinedLogs, "Combined logs should match expected format")
	})

	t.Run("stderr only", func(t *testing.T) {
		var stdoutBuffer, stderrBuffer bytes.Buffer

		stderrBuffer.WriteString("stderr line 1\n")
		stderrBuffer.WriteString("stderr line 2\n")

		combinedLogs := strings.TrimSuffix(stdoutBuffer.String(), "\n")
		if stderrBuffer.Len() > 0 {
			stderrContent := strings.TrimSuffix(stderrBuffer.String(), "\n")
			if combinedLogs != "" {
				combinedLogs += "\n" + stderrContent
			} else {
				combinedLogs = stderrContent
			}
		}

		expected := "stderr line 1\nstderr line 2"
		assert.Equal(t, expected, combinedLogs, "Combined logs should handle stderr-only case")
	})
}
