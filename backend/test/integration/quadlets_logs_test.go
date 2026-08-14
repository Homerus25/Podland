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

// Test_Integration_Quadlets_Logs is the quadlet logs vertical slice for the
// real-podman integration suite (PRD 080-10). It proves the quadlet→managed
// container resolution plus the two log delivery modes of
// GET /api/machines/{id}/quadlets/{quadletId}/logs:
//
//	non-streaming (follow=false, default) → 200 {id, logs} multiline string
//	streaming     (follow=true)            → NDJSON {timestamp, level, message} lines
//
// The quadlet must be running so its managed container exists and produces
// output — created and started here via the helpers from the 080-08/080-09
// slices. Unlike those slices' sleep-forever recipe, this one uses a
// logging recipe (echo loop) so the log assertions are meaningful.
//
// Isolation/cleanup model (matching the CRUD + lifecycle slices): the
// quadlet is created with a run-scoped name (it-<runID>-logs); t.Cleanup
// force-stops + force-deletes the quadlet via the API and force-removes
// the systemd-spawned container over the podman connection. The suite
// sweep (registerQuadletsSweep) is the abort backstop.
func Test_Integration_Quadlets_Logs(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// The logs path resolves the managed container through podman's
	// QuadletList over the SSH tunnel — same transport preconditions as
	// the lifecycle slice (user systemd must be usable for the quadlet
	// to run at all).
	requireUserSystemd(t)

	// Quadlet start does not auto-pull; busybox must pre-exist.
	ensureBusyboxImage(t)

	// Run-scoped name: file on the remote machine becomes
	// `<name>.container`, the systemd unit `<name>.service`, and the
	// spawned container `systemd-<name>` (podman quadlet naming).
	quadletName := "it-" + suite.RunID + "-logs"
	quadletID := quadletName + ".container"

	// Per-test cleanup, registered before create so it runs even if a
	// later phase fails. Best-effort; the sweep is the abort backstop.
	t.Cleanup(func() {
		forceDeleteQuadlet(t, machineID, quadletID)
		removeSpawnedContainer(t, quadletName)
	})

	// --- Setup: create + start the logging quadlet -------------------
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		Content:  loggingContainerQuadletContent,
	})
	startQuadlet(t, machineID, quadletID)
	waitForQuadletRunning(t, machineID, quadletID,
		"logs slice setup: quadlet must be running to have a managed container")

	// --- AC: non-streaming GET .../logs returns 200 {id, logs} --------
	t.Run("NonStreamingReturnsLogsWithId", func(t *testing.T) {
		logs := waitForQuadletLogOutput(t, machineID, quadletID)
		require.Equal(t, quadletID, logs.ID,
			"logs response id must echo the quadlet id from the URL path")
		require.Contains(t, logs.Logs, "hi",
			"logs must contain the recipe's stdout output. got: %q", logs.Logs)
	})

	// --- AC: since (RFC3339) and tail query params accepted ------------
	t.Run("SinceAndTailParamsAccepted", func(t *testing.T) {
		// since one hour ago: everything the container emitted qualifies.
		since := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		all := getQuadletLogs(t, machineID, quadletID, "since="+url.QueryEscape(since))
		require.Contains(t, all.Logs, "hi", "logs with since=<1h ago> must include the recipe's output")

		// tail=1: podman keeps only the most recent line.
		last := getQuadletLogs(t, machineID, quadletID, "tail=1")
		require.Equal(t, 1, len(strings.Split(strings.TrimSuffix(last.Logs, "\n"), "\n")),
			"tail=1 must return exactly one log line. got: %q", last.Logs)
		require.Equal(t, "hi", strings.TrimSuffix(last.Logs, "\n"),
			"the tailed line must be the recipe's output")
	})

	// --- AC: follow=true streams NDJSON {timestamp, level, message} ----
	t.Run("StreamingFollowReturnsNDJSONEvents", func(t *testing.T) {
		resp, cancel := openQuadletLogStream(t, machineID, quadletID)
		defer cancel()
		defer resp.Body.Close()

		// The NDJSON stream is served by the shared streaming helper
		// (streaming.SetNDJSONHeaders), which pins Content-Type to
		// application/json across the images/containers/quadlets domains
		// (unit-tested in streaming/ndjson_test.go). The PRD calls it
		// "application/x-ndjson"; the wire contract is the shared helper's.
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json",
			"the NDJSON stream must be served as JSON")

		scanner := bufio.NewScanner(resp.Body)
		require.Truef(t, scanner.Scan(),
			"follow stream must emit at least one NDJSON line (scan err: %v)", scanner.Err())
		var events []quadletLogEventLine
		decodeLogEventLine(t, scanner.Text(), &events)
		// A follow stream must keep producing: the recipe emits every
		// second, so a second line proves streaming, not a one-shot dump.
		require.Truef(t, scanner.Scan(),
			"follow stream must emit a second NDJSON line (scan err: %v)", scanner.Err())
		decodeLogEventLine(t, scanner.Text(), &events)
		require.GreaterOrEqual(t, len(events), 2, "at least two events must have been decoded")
	})

	// --- AC: client disconnect during follow=true cancels the stream ---
	t.Run("ClientDisconnectCancelsFollowStream", func(t *testing.T) {
		resp, cancel := openQuadletLogStream(t, machineID, quadletID)
		defer cancel()

		// Prove the stream is live before disconnecting.
		scanner := bufio.NewScanner(resp.Body)
		require.Truef(t, scanner.Scan(),
			"follow stream must emit a line before the disconnect (err: %v)", scanner.Err())

		// Disconnect: cancel the request context. Server-side the request
		// context cancels, the handler emits its OPERATION_CANCELLED event
		// to the dead connection, and GetLogsWithStream must return so the
		// machine connection is released. That event is written to a
		// connection the client has already dropped, so it is not directly
		// observable from here — the behavioral proof is that reading the
		// stream terminates promptly and the logs path stays fully usable
		// for a brand-new follow stream immediately after.
		cancel()
		require.Truef(t, drainScanner(scanner),
			"reading a canceled follow stream must terminate promptly, not hang")
		resp.Body.Close()

		resp2, cancel2 := openQuadletLogStream(t, machineID, quadletID)
		defer cancel2()
		scanner2 := bufio.NewScanner(resp2.Body)
		require.Truef(t, scanner2.Scan(),
			"a new follow stream must work right after the disconnect — "+
				"the canceled stream must have released the machine connection (err: %v)",
			scanner2.Err())
		cancel2()
		resp2.Body.Close()
	})

	// --- AC: logs on a missing quadlet returns 404 QUADLET_NOT_FOUND ---
	t.Run("MissingQuadletReturns404", func(t *testing.T) {
		missing := "it-" + suite.RunID + "-missing.container"
		status, raw := getQuadletLogsRaw(t, machineID, missing, "")
		requireQuadletLogs404(t, status, raw, "logs on a missing quadlet")
	})

	// --- AC: quadlet whose managed container doesn't exist → 404 -------
	//
	// Stopping the quadlet removes the systemd-spawned container while the
	// quadlet file itself remains — exactly the "managed container doesn't
	// exist" case. Runs last: it leaves the quadlet stopped (cleanup
	// force-deletes it either way).
	t.Run("StoppedQuadletManagedContainerGoneReturns404", func(t *testing.T) {
		quadletLifecycleAction(t, machineID, quadletID, "stop",
			url.Values{"force": {"true"}})
		waitForQuadletNotRunning(t, machineID, quadletID,
			"stop must transition the quadlet out of running before the logs 404 check")

		status, raw := getQuadletLogsRaw(t, machineID, quadletID, "")
		requireQuadletLogs404(t, status, raw, "logs on a quadlet without a managed container")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// loggingContainerQuadletContent is the recipe this slice runs: a busybox
// container that emits "hi" to stdout once per second, so both the
// non-streaming and the follow-stream assertions have real output to read.
const loggingContainerQuadletContent = `[Container]
Image=busybox
Exec=sh -c "while true; do echo hi; sleep 1; done"
`

// quadletLogsResponse mirrors the fields of quadlets.QuadletLogsResponse
// that this slice consumes.
type quadletLogsResponse struct {
	ID   string `json:"id"`
	Logs string `json:"logs"`
}

// getQuadletLogsRaw GETs /quadlets/{id}/logs (with optional query params)
// and returns the raw status and body without asserting either.
func getQuadletLogsRaw(t *testing.T, machineID, quadletID string, query string) (int, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletID + "/logs"
	if query != "" {
		u += "?" + query
	}
	resp, err := suite.AuthenticatedClient().Get(u)
	require.NoError(t, err, "GET /quadlets/{id}/logs must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// requireQuadletLogs404 asserts the logs endpoint's not-found contract:
// HTTP 404 with a QUADLET_NOT_FOUND error envelope — shared by the
// missing-quadlet and managed-container-gone cases.
func requireQuadletLogs404(t *testing.T, status int, raw []byte, msg string) {
	t.Helper()
	require.Equalf(t, http.StatusNotFound, status,
		"%s: expected 404, got %d. body: %s", msg, status, string(raw))
	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", msg, string(raw))
	require.Equal(t, "QUADLET_NOT_FOUND", errResp.Error.Code,
		"%s: error code must be QUADLET_NOT_FOUND", msg)
}

// getQuadletLogs GETs /quadlets/{id}/logs and asserts 200 + the
// {id, logs} response shape, returning the decoded body.
func getQuadletLogs(t *testing.T, machineID, quadletID, query string) quadletLogsResponse {
	t.Helper()
	status, raw := getQuadletLogsRaw(t, machineID, quadletID, query)
	require.Equalf(t, http.StatusOK, status,
		"get quadlet logs: expected 200, got %d. body: %s", status, string(raw))
	var logs quadletLogsResponse
	require.NoErrorf(t, json.Unmarshal(raw, &logs),
		"get quadlet logs: response must decode as {id, logs}. body: %s", string(raw))
	return logs
}

// logOutputPollTimeout caps how long waitForQuadletLogOutput polls for the
// managed container's first output. Bounded generously against quadlet
// start lag, but short enough that a resolution failure fails fast.
const logOutputPollTimeout = 30 * time.Second

// waitForQuadletLogOutput polls the non-streaming logs endpoint until it
// returns 200 with non-empty logs. The container needs a moment after the
// quadlet reports running before its first "hi" lands in the log stream;
// a 404 here means the managed container resolution failed.
func waitForQuadletLogOutput(t *testing.T, machineID, quadletID string) quadletLogsResponse {
	t.Helper()
	deadline := time.Now().Add(logOutputPollTimeout)
	var lastBody []byte
	var lastStatus int
	for time.Now().Before(deadline) {
		lastStatus, lastBody = getQuadletLogsRaw(t, machineID, quadletID, "")
		if lastStatus == http.StatusOK {
			var logs quadletLogsResponse
			if err := json.Unmarshal(lastBody, &logs); err == nil && logs.Logs != "" {
				return logs
			}
		}
		time.Sleep(quadletPollInterval)
	}
	require.Failf(t, "quadlet logs never became available",
		"GET .../%s/logs did not return non-empty logs within %s (last status=%d, body=%s)",
		quadletID, logOutputPollTimeout, lastStatus, string(lastBody))
	return quadletLogsResponse{}
}

// ---------------------------------------------------------------------------
// Streaming (follow=true) helpers
// ---------------------------------------------------------------------------

// quadletLogEventLine mirrors the fields of quadlets.QuadletLogEvent that
// this slice consumes. Error/cancellation events additionally carry a
// `code`, so it is captured too.
type quadletLogEventLine struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
}

// decodeLogEventLine decodes one NDJSON stream line as a log event,
// asserts the event shape, and appends it to events.
func decodeLogEventLine(t *testing.T, line string, events *[]quadletLogEventLine) {
	t.Helper()
	line = strings.TrimSpace(line)
	require.NotEmptyf(t, line, "NDJSON stream must not contain blank event lines")

	var ev quadletLogEventLine
	require.NoErrorf(t, json.Unmarshal([]byte(line), &ev),
		"NDJSON stream line must decode as {timestamp, level, message}. line: %s", line)
	require.NotZerof(t, ev.Timestamp, "log event must carry a timestamp. line: %s", line)
	_, err := time.Parse(time.RFC3339, ev.Timestamp)
	require.NoErrorf(t, err, "log event timestamp must be RFC3339. line: %s", line)
	require.NotZerof(t, ev.Level, "log event must carry a level. line: %s", line)
	require.NotZerof(t, ev.Message, "log event must carry a message. line: %s", line)
	*events = append(*events, ev)
}

// streamDrainTimeout bounds how long drainScanner may take. A canceled
// follow stream must terminate reading promptly; the bound turns a hang
// into a test failure instead of a suite timeout.
const streamDrainTimeout = 30 * time.Second

// drainScanner keeps scanning until the stream ends. It returns false if
// the stream did not terminate within streamDrainTimeout. Lines already
// buffered client-side when a stream is canceled may still come out — the
// requirement under test is termination, not which buffered lines arrive.
func drainScanner(scanner *bufio.Scanner) bool {
	drained := make(chan struct{})
	go func() {
		for scanner.Scan() {
		}
		close(drained)
	}()
	select {
	case <-drained:
		return true
	case <-time.After(streamDrainTimeout):
		return false
	}
}

// openQuadletLogStream opens a follow=true log stream and returns the live
// response plus the cancel func for the request context.
//
// The request mirrors pullImageStreamToCompletion from the images slice:
// the Bearer header is set by hand and a dedicated client with no Timeout
// is used, because http.Client's Timeout covers the entire exchange
// including body reads — which would cut off a follow stream (the default
// AuthenticatedClient carries a 30s timeout). The caller controls the
// stream lifetime via the returned cancel func.
//
// The streaming client uses its OWN transport (a clone of the default):
// the suite's AuthenticatedClient pools on http.DefaultTransport, and a
// canceled follow stream returns connections to the pool with leftover
// chunked data — polluting the next regular request that reuses them.
var quadletLogStreamClient = &http.Client{
	Transport: http.DefaultTransport.(*http.Transport).Clone(),
}

func openQuadletLogStream(t *testing.T, machineID, quadletID string) (*http.Response, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	u := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletID + "/logs?follow=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+suite.Token)

	resp, err := quadletLogStreamClient.Do(req)
	require.NoErrorf(t, err, "GET .../logs?follow=true must connect to the in-process server")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"follow stream must return 200 to begin the NDJSON stream. status=%d", resp.StatusCode)
	return resp, cancel
}
