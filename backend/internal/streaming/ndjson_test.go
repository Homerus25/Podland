package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetNDJSONHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SetNDJSONHeaders(w)

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
	if cacheControl := w.Header().Get("Cache-Control"); cacheControl != "no-cache" {
		t.Errorf("Expected Cache-Control 'no-cache', got '%s'", cacheControl)
	}
	if connection := w.Header().Get("Connection"); connection != "keep-alive" {
		t.Errorf("Expected Connection 'keep-alive', got '%s'", connection)
	}
	if transferEncoding := w.Header().Get("Transfer-Encoding"); transferEncoding != "chunked" {
		t.Errorf("Expected Transfer-Encoding 'chunked', got '%s'", transferEncoding)
	}
}

func TestNewNDJSONWriter(t *testing.T) {
	w := httptest.NewRecorder()
	writer, err := NewNDJSONWriter(w)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if writer == nil {
		t.Fatal("Expected writer to be non-nil")
	}
}

func TestNewNDJSONWriter_NonFlusher(t *testing.T) {
	w := &nonFlusher{}
	_, err := NewNDJSONWriter(w)
	if err == nil {
		t.Fatal("Expected error for non-flusher, got nil")
	}
}

func TestNDJSONWriter_Write(t *testing.T) {
	w := httptest.NewRecorder()
	writer, err := NewNDJSONWriter(w)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	testData := map[string]string{"message": "hello", "status": "ok"}
	err = writer.Write(testData)
	if err != nil {
		t.Fatalf("Expected no error writing, got %v", err)
	}

	output := w.Body.String()
	if output == "" {
		t.Fatal("Expected output to be non-empty")
	}
	if output[len(output)-1] != '\n' {
		t.Error("Expected output to end with newline")
	}
}

func TestDisconnectChan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	disconnectChan := DisconnectChan(ctx)

	select {
	case <-disconnectChan:
		t.Fatal("Channel should not be closed immediately")
	default:
	}

	cancel()

	select {
	case <-disconnectChan:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Channel should be closed after context cancellation")
	}
}

type nonFlusher struct{}

func (n *nonFlusher) Header() http.Header {
	return http.Header{}
}

func (n *nonFlusher) Write([]byte) (int, error) {
	return 0, nil
}

func (n *nonFlusher) WriteHeader(int) {}
