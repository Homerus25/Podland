//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Containers_Logs is the containers logs vertical slice
// for the real-podman integration suite, mirroring the quadlets logs
// slice. It proves the two log delivery modes of
// GET /api/hosts/{id}/containers/{cid}/logs against a real container:
//
//	non-streaming (follow=false, default) → 200 {id, logs} multiline string
//	streaming     (follow=true)            → NDJSON {timestamp, level, message} lines
//
// The container must be running and producing output — created and
// started here with an echo-loop recipe so the log assertions are
// meaningful.
//
// Isolation/cleanup model (matching the containers lifecycle slice): the
// container is created with a run-scoped name (it-<runID>-logs); t.Cleanup
// force-removes it over the podman connection, and the suite sweep
// (registerContainersSweep) is the abort backstop.
func Test_Integration_Containers_Logs(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerContainersSweep()

	hostID := suite.HostID

	// Container create does not auto-pull; busybox must pre-exist.
	ensureBusyboxImage(t)

	// Run-scoped name so assertions and the suite sweep find exactly what
	// this run created.
	containerName := "it-" + suite.RunID + "-logs"

	// Per-test cleanup, registered before create so it runs even if a
	// later phase fails. Best-effort; the sweep is the abort backstop.
	var containerID string
	t.Cleanup(func() {
		if containerID == "" {
			return
		}
		if out, err := podmanConnRun("rm", "-f", containerID); err != nil {
			t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", containerID, err, strings.TrimSpace(out))
		}
	})

	// --- Setup: create + start the logging container ------------------
	createReq := containerCreateBody{
		Name:    containerName,
		Image:   "busybox:latest",
		Command: []string{"sh", "-c", "while true; do echo hi; sleep 1; done"},
	}
	containerID = createContainer(t, hostID, createReq)
	require.NotEmpty(t, containerID, "create must return a non-empty container id")
	startContainer(t, hostID, containerID)
	waitForContainerState(t, hostID, containerID, "running",
		"logs slice setup: container must be running to produce output")

	// --- AC: non-streaming GET .../logs returns 200 {id, logs} --------
	t.Run("NonStreamingReturnsLogsWithId", func(t *testing.T) {
		logs := waitForContainerLogOutput(t, hostID, containerID)
		require.Equal(t, containerID, logs.ID,
			"logs response id must echo the container id from the URL path")
		require.Contains(t, logs.Logs, "hi",
			"logs must contain the recipe's stdout output. got: %q", logs.Logs)
	})

	// --- AC: since (RFC3339) and tail query params accepted ------------
	t.Run("SinceAndTailParamsAccepted", func(t *testing.T) {
		// since one hour ago: everything the container emitted qualifies.
		since := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		all := getContainerLogs(t, hostID, containerID, "since="+url.QueryEscape(since))
		require.Contains(t, all.Logs, "hi", "logs with since=<1h ago> must include the recipe's output")

		// tail=1: podman keeps only the most recent line.
		last := getContainerLogs(t, hostID, containerID, "tail=1")
		require.Equal(t, 1, len(strings.Split(strings.TrimSuffix(last.Logs, "\n"), "\n")),
			"tail=1 must return exactly one log line. got: %q", last.Logs)
		require.Equal(t, "hi", strings.TrimSuffix(last.Logs, "\n"),
			"the tailed line must be the recipe's output")
	})

	// --- AC: follow=true streams NDJSON {timestamp, level, message} ----
	t.Run("StreamingFollowReturnsNDJSONEvents", func(t *testing.T) {
		resp, cancel := openContainerLogStream(t, hostID, containerID)
		defer cancel()
		defer resp.Body.Close()

		// The NDJSON stream is served by the streaming kit's shared
		// header helper, which pins Content-Type to
		// application/json across the images/containers/quadlets domains
		// (unit-tested in streaming/ndjson_test.go).
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json",
			"the NDJSON stream must be served as JSON")

		scanner := bufio.NewScanner(resp.Body)
		require.Truef(t, scanner.Scan(),
			"follow stream must emit at least one NDJSON line (scan err: %v)", scanner.Err())
		var events []containerLogEventLine
		decodeContainerLogEventLine(t, scanner.Text(), &events)
		// A follow stream must keep producing: the recipe emits every
		// second, so a second line proves streaming, not a one-shot dump.
		require.Truef(t, scanner.Scan(),
			"follow stream must emit a second NDJSON line (scan err: %v)", scanner.Err())
		decodeContainerLogEventLine(t, scanner.Text(), &events)
		require.GreaterOrEqual(t, len(events), 2, "at least two events must have been decoded")
	})

	// --- AC: client disconnect during follow=true cancels the stream ---
	t.Run("ClientDisconnectCancelsFollowStream", func(t *testing.T) {
		resp, cancel := openContainerLogStream(t, hostID, containerID)
		defer cancel()

		// Prove the stream is live before disconnecting.
		scanner := bufio.NewScanner(resp.Body)
		require.Truef(t, scanner.Scan(),
			"follow stream must emit a line before the disconnect (err: %v)", scanner.Err())

		// Disconnect: cancel the request context. Server-side the request
		// context cancels, the kit emits its OPERATION_CANCELLED event to
		// the dead connection, and GetLogsWithStream must return so the
		// host connection is released. That event is written to a
		// connection the client has already dropped, so it is not directly
		// observable from here — the behavioral proof is that reading the
		// stream terminates promptly and the logs path stays fully usable
		// for a brand-new follow stream immediately after.
		cancel()
		require.Truef(t, drainScanner(scanner),
			"reading a canceled follow stream must terminate promptly, not hang")
		resp.Body.Close()

		resp2, cancel2 := openContainerLogStream(t, hostID, containerID)
		defer cancel2()
		scanner2 := bufio.NewScanner(resp2.Body)
		require.Truef(t, scanner2.Scan(),
			"a new follow stream must work right after the disconnect — "+
				"the canceled stream must have released the host connection (err: %v)",
			scanner2.Err())
		cancel2()
		resp2.Body.Close()
	})

	// --- AC: logs on a missing container returns 404 -------------------
	t.Run("MissingContainerReturns404", func(t *testing.T) {
		missing := "it-" + suite.RunID + "-missing"
		status, raw := getContainerLogsRaw(t, hostID, missing, "")
		requireContainerLogs404(t, status, raw, "logs on a missing container")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// containerLogsResponse mirrors the fields of containers.ContainerLogsResponse
// that this slice consumes.
type containerLogsResponse struct {
	ID   string `json:"id"`
	Logs string `json:"logs"`
}

// getContainerLogsRaw GETs /containers/{id}/logs (with optional query
// params) and returns the raw status and body without asserting either.
func getContainerLogsRaw(t *testing.T, hostID, containerID string, query string) (int, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/hosts/" + hostID + "/containers/" + containerID + "/logs"
	if query != "" {
		u += "?" + query
	}
	resp, err := suite.AuthenticatedClient().Get(u)
	require.NoError(t, err, "GET /containers/{id}/logs must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// requireContainerLogs404 asserts the logs endpoint's not-found contract:
// HTTP 404 with a CONTAINER_NOT_FOUND error envelope.
func requireContainerLogs404(t *testing.T, status int, raw []byte, msg string) {
	t.Helper()
	require.Equalf(t, http.StatusNotFound, status,
		"%s: expected 404, got %d. body: %s", msg, status, string(raw))
	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", msg, string(raw))
	require.Equal(t, "CONTAINER_NOT_FOUND", errResp.Error.Code,
		"%s: error code must be CONTAINER_NOT_FOUND", msg)
}

// getContainerLogs GETs /containers/{id}/logs and asserts 200 + the
// {id, logs} response shape, returning the decoded body.
func getContainerLogs(t *testing.T, hostID, containerID, query string) containerLogsResponse {
	t.Helper()
	status, raw := getContainerLogsRaw(t, hostID, containerID, query)
	require.Equalf(t, http.StatusOK, status,
		"get container logs: expected 200, got %d. body: %s", status, string(raw))
	var logs containerLogsResponse
	require.NoErrorf(t, json.Unmarshal(raw, &logs),
		"get container logs: response must decode as {id, logs}. body: %s", string(raw))
	return logs
}

// waitForContainerLogOutput polls the non-streaming logs endpoint until it
// returns 200 with non-empty logs. The container needs a moment after the
// start reports running before its first "hi" lands in the log stream.
func waitForContainerLogOutput(t *testing.T, hostID, containerID string) containerLogsResponse {
	t.Helper()
	deadline := time.Now().Add(logOutputPollTimeout)
	var lastBody []byte
	var lastStatus int
	for time.Now().Before(deadline) {
		lastStatus, lastBody = getContainerLogsRaw(t, hostID, containerID, "")
		if lastStatus == http.StatusOK {
			var logs containerLogsResponse
			if err := json.Unmarshal(lastBody, &logs); err == nil && logs.Logs != "" {
				return logs
			}
		}
		time.Sleep(quadletPollInterval)
	}
	require.Failf(t, "container logs never became available",
		"GET .../%s/logs did not return non-empty logs within %s (last status=%d, body=%s)",
		containerID, logOutputPollTimeout, lastStatus, string(lastBody))
	return containerLogsResponse{}
}

// ---------------------------------------------------------------------------
// Streaming (follow=true) helpers
// ---------------------------------------------------------------------------

// containerLogEventLine mirrors the fields of streaming.LogEvent that this
// slice consumes. Error/cancellation events additionally carry a `code`,
// so it is captured too.
type containerLogEventLine struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
}

// decodeContainerLogEventLine decodes one NDJSON stream line as a log
// event, asserts the event shape, and appends it to events.
func decodeContainerLogEventLine(t *testing.T, line string, events *[]containerLogEventLine) {
	t.Helper()
	line = strings.TrimSpace(line)
	require.NotEmptyf(t, line, "NDJSON stream must not contain blank event lines")

	var ev containerLogEventLine
	require.NoErrorf(t, json.Unmarshal([]byte(line), &ev),
		"NDJSON stream line must decode as {timestamp, level, message}. line: %s", line)
	require.NotZerof(t, ev.Timestamp, "log event must carry a timestamp. line: %s", line)
	_, err := time.Parse(time.RFC3339, ev.Timestamp)
	require.NoErrorf(t, err, "log event timestamp must be RFC3339. line: %s", line)
	require.NotZerof(t, ev.Level, "log event must carry a level. line: %s", line)
	require.NotZerof(t, ev.Message, "log event must carry a message. line: %s", line)
	*events = append(*events, ev)
}

// containerLogStreamClient mirrors quadletLogStreamClient: no Timeout
// (which would cut off a follow stream mid-body) and its OWN transport
// (a canceled follow stream returns connections to the pool with leftover
// chunked data — polluting the next regular request that reuses them).
var containerLogStreamClient = &http.Client{
	Transport: http.DefaultTransport.(*http.Transport).Clone(),
}

// openContainerLogStream opens a follow=true log stream and returns the
// live response plus the cancel func for the request context.
func openContainerLogStream(t *testing.T, hostID, containerID string) (*http.Response, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	u := suite.ServerURL + "/api/hosts/" + hostID + "/containers/" + containerID + "/logs?follow=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+suite.Token)

	resp, err := containerLogStreamClient.Do(req)
	require.NoErrorf(t, err, "GET .../logs?follow=true must connect to the in-process server")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"follow stream must return 200 to begin the NDJSON stream. status=%d", resp.StatusCode)
	return resp, cancel
}
