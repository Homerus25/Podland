package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SetNDJSONHeaders sets the standard headers for NDJSON streaming responses
func SetNDJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
}

// NDJSONWriter handles writing NDJSON (newline-delimited JSON) responses with flushing
type NDJSONWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

// NewNDJSONWriter creates a new NDJSONWriter after asserting the response writer supports flushing
func NewNDJSONWriter(w http.ResponseWriter) (*NDJSONWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	return &NDJSONWriter{
		writer:  w,
		flusher: flusher,
	}, nil
}

// Write marshals the value as JSON, writes it as a newline-delimited line, and flushes
func (nw *NDJSONWriter) Write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(nw.writer, string(data)); err != nil {
		return err
	}
	nw.flusher.Flush()
	return nil
}

// DisconnectChan returns a channel that closes when the context is done
func DisconnectChan(ctx context.Context) <-chan struct{} {
	disconnectChan := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(disconnectChan)
	}()
	return disconnectChan
}