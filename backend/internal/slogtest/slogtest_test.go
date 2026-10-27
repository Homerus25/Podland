package slogtest

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogCapture pins the capture contract once for every consuming suite:
// records arrive in order with their level, Attrs filters by level and
// renders error values as text, and CaptureDefault restores the previous
// default logger.
func TestLogCapture(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	capture := CaptureDefault(t)

	slog.Warn("warn line", slog.String("op", "containers.list"), slog.Any("cause", cause))
	slog.Error("error line", slog.String("requestID", "req-1"))

	records := capture.Records()
	require.Len(t, records, 2, "every handled record is captured, in order")
	assert.Equal(t, "warn line", records[0].Message)
	assert.Equal(t, slog.LevelWarn, records[0].Level)
	assert.Equal(t, "error line", records[1].Message)

	warns := capture.Attrs(t, slog.LevelWarn)
	require.Len(t, warns, 1, "Attrs filters to the requested level")
	assert.Equal(t, "containers.list", warns[0]["op"])
	assert.Equal(t, cause.Error(), warns[0]["cause"],
		"error values are rendered as their text")

	errs := capture.Attrs(t, slog.LevelError)
	require.Len(t, errs, 1)
	assert.Equal(t, "req-1", errs[0]["requestID"])
}
