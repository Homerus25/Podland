package middleware

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequestLogging_ForwardsWriteDeadlineExtension pins the production
// composition behind every long stream: the server's WriteTimeout
// (cmd/server/main.go) is only survivable because the streaming kit
// extends the per-response write deadline (internal/streaming), and that
// extension must survive this middleware's ResponseWriter wrapper. The
// operation outlasts the deadline the way a real quadlet auto-update
// outlasts the 30-second server timeout: if the extension is lost, the
// terminal event's write fails and the client never sees the run finish.
func TestRequestLogging_ForwardsWriteDeadlineExtension(t *testing.T) {
	const writeTimeout = 300 * time.Millisecond

	streamEndpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := contextutils.SetUserID(r.Context(), "test-user")
		r = r.WithContext(ctx)
		r.SetPathValue("id", "test-machine")
		streaming.HandleProgress(w, r, "test-op", nil,
			streaming.ProgressConfig[int, string]{
				Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(int)) (string, error) {
					onProgress(1)
					select {
					case <-time.After(2 * writeTimeout):
					case <-ctx.Done():
					}
					return "done", nil
				},
				CompleteEvent: func(op streaming.OperationContext, result string) any { return result },
				ErrorEvent:    func(op streaming.OperationContext, code, message string, cancelled bool) any { return code },
			})
	})

	srv := httptest.NewUnstartedServer(RequestLogging(streamEndpoint))
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	require.Len(t, lines, 2,
		"the client must receive the progress event and the terminal complete event even when the operation outlasts the server WriteTimeout; got %q", lines)
	assert.Equal(t, "1", lines[0])
	assert.Equal(t, `"done"`, lines[1])
}
