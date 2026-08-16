//go:build integration

package integration

import (
	"bufio"
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
func Test_Integration_Quadlets_Logs_ExplicitName(t *testing.T) {
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
		Content:  explicitNameContainerQuadletContent(loggingContainerQuadletContent),
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
