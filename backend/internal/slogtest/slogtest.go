// Package slogtest is the one owner of the slog capture handler tests use
// to pin server-side log lines (level, message, attrs) without reading
// stderr. The third copy of the capture-handler shape (connections,
// streaming, hosts — issue 399) earned the extraction
// (docs/backend-conventions.md, cross-domain duplication ruling). It
// lives in its own leaf package because internal/testutils imports
// internal/connections, so hosting it there would be an import cycle for
// the connections suite.
package slogtest

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// LogCapture is a slog.Handler that records every record it handles.
type LogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *LogCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *LogCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *LogCapture) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *LogCapture) WithGroup(string) slog.Handler { return h }

// CaptureDefault installs a LogCapture as the default logger for the
// test's duration (restored via t.Cleanup) and returns it.
func CaptureDefault(t *testing.T) *LogCapture {
	t.Helper()
	capture := &LogCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return capture
}

// Records returns the captured records in the order they were handled.
func (h *LogCapture) Records() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

// Attrs returns the attrs of every record at the given level, one map per
// record, with error values rendered as their text.
func (h *LogCapture) Attrs(t *testing.T, level slog.Level) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, r := range h.Records() {
		if r.Level != level {
			continue
		}
		attrs := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			if err, ok := a.Value.Any().(error); ok {
				attrs[a.Key] = err.Error()
				return true
			}
			attrs[a.Key] = a.Value.String()
			return true
		})
		out = append(out, attrs)
	}
	return out
}
