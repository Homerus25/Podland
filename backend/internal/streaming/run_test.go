package streaming

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
)

func newTestWriter(t *testing.T) (*NDJSONWriter, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	w, err := NewNDJSONWriter(rec)
	if err != nil {
		t.Fatalf("NewNDJSONWriter: %v", err)
	}
	return w, rec
}

func decodeLines(t *testing.T, body string) []string {
	t.Helper()
	var lines []string
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			lines = append(lines, body[start:i])
			start = i + 1
		}
	}
	if start < len(body) {
		lines = append(lines, body[start:])
	}
	if len(lines) == 0 {
		t.Fatalf("expected at least one NDJSON line, body: %q", body)
	}
	return lines
}

func TestRunStreamsProgressAndCompletes(t *testing.T) {
	w, rec := newTestWriter(t)

	delivered := make(chan struct{}, 2)
	Run(context.Background(), w, StreamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			onProgress(1)
			onProgress(2)
			<-delivered
			<-delivered
			return "done", nil
		},
		OnEvent: func(event int) error {
			err := w.Write(map[string]any{"progress": event})
			delivered <- struct{}{}
			return err
		},
		CompleteEvent: func(result string) any { return map[string]any{"complete": result} },
		ErrorEvent:    func(err error, cancelled bool) any { return map[string]any{"error": err.Error()} },
	})

	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %q", len(lines), rec.Body.String())
	}
	want := []string{`{"progress":1}`, `{"progress":2}`, `{"complete":"done"}`}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: got %s, want %s", i, lines[i], want[i])
		}
	}
}

func TestRunWritesExactlyOneTerminalErrorEvent(t *testing.T) {
	w, rec := newTestWriter(t)
	terminal := errors.New("boom")

	Run(context.Background(), w, StreamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			onProgress(1)
			return "", terminal
		},
		OnEvent:       func(event int) error { return w.Write(event) },
		CompleteEvent: func(result string) any { return map[string]any{"complete": result} },
		ErrorEvent: func(err error, cancelled bool) any {
			if cancelled {
				t.Fatal("unexpected cancelled=true for non-context error")
			}
			if !errors.Is(err, terminal) {
				t.Fatalf("got err %v, want %v", err, terminal)
			}
			return map[string]any{"error": err.Error()}
		},
	})

	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines (progress + one error), got %d: %q", len(lines), rec.Body.String())
	}
	if lines[1] != `{"error":"boom"}` {
		t.Fatalf("terminal line: got %s", lines[1])
	}
}

func TestRunCancelsOperationOnClientDisconnect(t *testing.T) {
	w, rec := newTestWriter(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	operationCancelled := false
	Run(ctx, w, StreamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			defer wg.Done()
			<-ctx.Done()
			operationCancelled = true
			return "", ctx.Err()
		},
		OnEvent:       func(event int) error { return nil },
		CompleteEvent: func(result string) any { return result },
		ErrorEvent: func(err error, cancelled bool) any {
			if !cancelled {
				t.Fatal("disconnect must report cancelled=true")
			}
			return map[string]any{"error": "cancelled"}
		},
	})
	wg.Wait()
	if !operationCancelled {
		t.Fatal("operation was not cancelled on disconnect")
	}

	lines := decodeLines(t, rec.Body.String())
	if len(lines) != 1 {
		t.Fatalf("expected exactly one terminal event, got %d: %q", len(lines), rec.Body.String())
	}
	if lines[0] != `{"error":"cancelled"}` {
		t.Fatalf("terminal line: got %s", lines[0])
	}
}

func TestRunStopsWithoutTerminalEventWhenWriteFails(t *testing.T) {
	w, rec := newTestWriter(t)

	completed := false
	finished := make(chan struct{})
	Run(context.Background(), w, StreamConfig[int, string]{
		Operation: func(ctx context.Context, onProgress func(int)) (string, error) {
			defer close(finished)
			onProgress(1)
			<-ctx.Done()
			completed = ctx.Err() != nil
			return "done", nil
		},
		OnEvent: func(event int) error {
			return errors.New("write failed")
		},
		CompleteEvent: func(result string) any { return result },
		ErrorEvent:    func(err error, cancelled bool) any { return err },
	})

	<-finished
	if rec.Body.String() != "" {
		t.Fatalf("no terminal event should be written after write failure, body: %q", rec.Body.String())
	}
	if !completed {
		t.Fatal("operation should have been cancelled after write failure")
	}
}
