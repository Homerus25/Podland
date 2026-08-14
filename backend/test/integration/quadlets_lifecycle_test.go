//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Test_Integration_Quadlets_Lifecycle is the quadlet lifecycle vertical
// slice for the real-podman integration suite (PRD 080-09). It proves the
// SSH-exec + systemd path end-to-end — the concern the 080 master
// explicitly deferred to this slice:
//
//	POST /api/machines/{id}/quadlets/{name}/start   → 200, unit reaches running
//	POST /api/machines/{id}/quadlets/{name}/stop    → 200, unit leaves running
//	POST /api/machines/{id}/quadlets/{name}/restart → 200, unit back to running
//
// These endpoints run `systemctl --user <action> <unit>` over SSH via the
// sshexec service wired into app.New, so this is the only slice that
// exercises the SSH-exec infrastructure against the real machine.
//
// State verification policy (same reasoning as waitForContainerState in
// the containers slice): every transition is verified by polling
// GET /quadlets/{name} — the QuadletList-derived `state` field parsed
// from the systemd unit status. The lifecycle response bodies hardcode
// their `state` field (see lifecycle.go buildResponse) and would mask a
// transition that never actually happened.
//
// State vocabulary reality check: parseStateFromStatus maps the systemd
// ActiveState/SubState pair ("active/running" / "inactive/dead") reported
// by podman's QuadletList. A started unit reports "active/running" →
// state "running". A stopped unit reports "inactive/dead", which contains
// neither "running" nor "stopped" → state "unknown". The stop transition
// is therefore verified as "state is no longer running" rather than
// waiting for a literal "stopped" value that parseStateFromStatus never
// produces for a plain stopped service.
//
// TRIPWIRES (pinned gaps, clearly commented at the assertion sites):
//
//   - DELETE on a running quadlet without `force` returns
//     QUADLET_RESOURCE_RUNNING (400) because Delete never wires the
//     `stop` param (the 080-08 gap this slice hosts).
//   - stop/restart with a `timeout` param fails because the service maps
//     timeout to `systemctl --kill-seconds=<n>`, which is not a systemctl
//     option at all (it's a podman-run flag) — systemctl exits 1 with
//     "unrecognized option" and the API surfaces a 500.
//
// If either tripwire assertion starts failing, the corresponding backend
// gap was fixed — update the test to assert the fixed behavior.
//
// Isolation/cleanup model (matching the CRUD slice): the quadlet is
// created with a run-scoped name (it-<runID>-life); t.Cleanup force-stops
// the unit, force-deletes the quadlet via the API and force-removes the
// systemd-spawned container over the podman connection. The suite sweep
// (registerQuadletsSweep) is the abort backstop.
func Test_Integration_Quadlets_Lifecycle(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// The lifecycle actions shell out to `systemctl --user` over SSH as
	// the machine's SSH user, so the machine must (a) accept SSH with the
	// registered identity and (b) have a functioning user systemd
	// instance. Rootless podman machines do; if the target can't, skip
	// with a clear reason rather than failing (per the PRD).
	requireUserSystemd(t)

	// Quadlet start does not auto-pull; busybox must pre-exist.
	ensureBusyboxImage(t)

	// Run-scoped name: the file on the remote machine becomes
	// `<name>.container`, the systemd unit `<name>.service`, and the
	// spawned container `systemd-<name>` (podman quadlet naming).
	quadletName := "it-" + suite.RunID + "-life"

	// Per-test cleanup, registered before create so it runs even if a
	// later phase fails. Best-effort; the sweep is the abort backstop.
	//
	// podman quadlet rm is not implemented for remote CLI clients (the
	// released CLI rejects quadlet subcommands over --connection), so
	// cleanup goes through the API (force delete stops a still-running
	// unit server-side) plus a CLI `rm -f` of the spawned container.
	t.Cleanup(func() {
		forceDeleteQuadlet(t, machineID, quadletName+".container")
		removeSpawnedContainer(t, quadletName)
	})

	// --- Setup: create the runnable quadlet --------------------------
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})

	// --- AC: lifecycle on a missing quadlet returns 404 --------------
	t.Run("LifecycleOnMissingQuadletReturns404", func(t *testing.T) {
		missing := "it-" + suite.RunID + "-missing.container"
		for _, action := range []string{"start", "stop", "restart"} {
			quadletLifecycleExpectError(t, machineID, missing, action, nil,
				http.StatusNotFound, "QUADLET_NOT_FOUND",
				"lifecycle on a missing quadlet")
		}
	})

	// --- AC: start transitions the quadlet to running ----------------
	t.Run("StartTransitionsToRunning", func(t *testing.T) {
		startQuadlet(t, machineID, quadletName+".container")
		waitForQuadletRunning(t, machineID, quadletName+".container",
			"start must transition the quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC: second start returns QUADLET_ALREADY_RUNNING (400) ------
	t.Run("SecondStartReturnsAlreadyRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, machineID, quadletName+".container", "start", nil,
			http.StatusBadRequest, "QUADLET_ALREADY_RUNNING",
			"second start on a running quadlet")
	})
	if t.Failed() {
		return
	}

	// --- AC (gap tripwire): DELETE on a running quadlet without force
	// returns QUADLET_RESOURCE_RUNNING (400) --------------------------
	//
	// GAP: the DELETE `stop` query param is a no-op — QuadletService.Delete
	// ignores it (see service.go), so the "stop-then-remove" path the
	// param was meant to trigger does not exist. Consequence: deleting a
	// running quadlet without `force` is refused with 400. Pinned here on
	// purpose (080-08 deferred the gap to this slice): if this assertion
	// starts failing, the stop-wiring was added — replace it with the
	// agreed stop-then-delete behavior.
	t.Run("DeleteOnRunningWithoutForceReturns400", func(t *testing.T) {
		status, raw := deleteQuadletWithQuery(t, machineID, quadletName+".container", nil)
		require.Equalf(t, http.StatusBadRequest, status,
			"delete running quadlet without force (pinned gap): expected 400, got %d. body: %s",
			status, string(raw))

		var errResp quadletErrorEnvelope
		require.NoErrorf(t, json.Unmarshal(raw, &errResp),
			"delete running quadlet (pinned gap): error response must decode. body: %s", string(raw))
		require.Equal(t, "QUADLET_RESOURCE_RUNNING", errResp.Error.Code,
			"delete running quadlet (pinned gap): error code must be QUADLET_RESOURCE_RUNNING")
	})
	if t.Failed() {
		return
	}

	// --- AC (gap tripwire): stop with a `timeout` param fails ---------
	//
	// GAP: LifecycleOptions.Timeout maps to `systemctl
	// --kill-seconds=<n>` (lifecycle.go buildLifecycleArgs), but
	// --kill-seconds is not a systemctl option on any systemd version
	// (it is a podman-run flag). systemctl exits 1 with "unrecognized
	// option" and the API surfaces a 500 INTERNAL_ERROR. Pinned here on
	// purpose: if this assertion starts failing, the timeout mechanism
	// was fixed — replace it with the agreed working behavior.
	t.Run("StopWithTimeoutParamFailsGap", func(t *testing.T) {
		quadletLifecycleExpectError(t, machineID, quadletName+".container", "stop",
			url.Values{"timeout": {"10"}},
			http.StatusInternalServerError, "INTERNAL_ERROR",
			"stop with timeout param (pinned gap: --kill-seconds is not a systemctl option)")
	})
	if t.Failed() {
		return
	}

	// --- AC: restart returns the quadlet to running; force accepted ---
	t.Run("RestartReturnsToRunning", func(t *testing.T) {
		restarted := quadletLifecycleAction(t, machineID, quadletName+".container", "restart",
			url.Values{"force": {"true"}})
		require.NotEmpty(t, restarted.RestartedAt,
			"restart response must include restartedAt")
		waitForQuadletRunning(t, machineID, quadletName+".container",
			"restart must return the quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC: stop transitions the quadlet out of running; force accepted
	t.Run("StopTransitionsOutOfRunning", func(t *testing.T) {
		quadletLifecycleAction(t, machineID, quadletName+".container", "stop",
			url.Values{"force": {"true"}})
		waitForQuadletNotRunning(t, machineID, quadletName+".container",
			"stop must transition the quadlet out of running")
	})
	if t.Failed() {
		return
	}

	// --- AC: second stop returns QUADLET_NOT_RUNNING (400) ------------
	t.Run("SecondStopReturnsNotRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, machineID, quadletName+".container", "stop", nil,
			http.StatusBadRequest, "QUADLET_NOT_RUNNING",
			"second stop on a non-running quadlet")
	})
	if t.Failed() {
		return
	}

	// --- AC: DELETE on a running quadlet with force=true returns 204 --
	//
	// Re-start first so the force is doing real work (deleting a stopped
	// quadlet would succeed without force too).
	t.Run("DeleteOnRunningWithForceReturns204", func(t *testing.T) {
		startQuadlet(t, machineID, quadletName+".container")
		waitForQuadletRunning(t, machineID, quadletName+".container",
			"re-start before force delete must reach running")

		status, raw := deleteQuadletWithQuery(t, machineID, quadletName+".container",
			url.Values{"force": {"true"}})
		require.Equalf(t, http.StatusNoContent, status,
			"delete running quadlet with force: expected 204, got %d. body: %s",
			status, string(raw))

		requireQuadletNotListed(t, machineID, quadletName+".container",
			"force-deleted quadlet must not appear in the quadlet list")
	})
}

// ---------------------------------------------------------------------------
// Prerequisite probe: user systemd over SSH
// ---------------------------------------------------------------------------

// requireUserSystemd verifies the machine's SSH target answers and can
// run `systemctl --user` — the transport every lifecycle action uses. It
// dials with the exact coordinates the backend registered the machine
// with (suite.goodMachineRegistration), so what it probes is what the
// sshexec service will actually hit. Skips (never fails) when the target
// can't support user systemd units, per the PRD.
func requireUserSystemd(t *testing.T) {
	t.Helper()
	target := suite.goodMachineRegistration

	key, err := ssh.ParsePrivateKey([]byte(target.SSHKey))
	if err != nil {
		t.Skipf("machine SSH identity unparseable, cannot run systemctl over SSH: %v", err)
	}

	cfg := &ssh.ClientConfig{
		User:            target.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // test fixture; matches sshexec dialer
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(target.Port)), cfg)
	if err != nil {
		t.Skipf("machine does not accept SSH (%v) — lifecycle actions cannot run", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Skipf("machine SSH refuses sessions (%v) — lifecycle actions cannot run", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput("systemctl --user is-system-running")
	status := strings.TrimSpace(string(out))
	// "degraded" still means the user manager is up; individual units may
	// have failed but systemctl itself works, which is all we need.
	if err != nil || (status != "running" && status != "degraded") {
		t.Skipf("machine user systemd unusable (is-system-running=%q, err=%v)", status, err)
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// quadletLifecycleResponse mirrors the fields of
// quadlets.QuadletLifecycleResponse that this slice consumes. The State
// field is the constant the handlers hardcode — it is asserted as
// response shape only, never as proof of a transition (see the slice
// header: GET polling is the source of truth).
type quadletLifecycleResponse struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	Unit        string `json:"unit"`
	RestartedAt string `json:"restartedAt,omitempty"`
}

// startQuadlet POSTs /quadlets/{name}/start and asserts 200 with the
// expected response shape.
func startQuadlet(t *testing.T, machineID, quadletName string) quadletLifecycleResponse {
	t.Helper()
	return quadletLifecycleAction(t, machineID, quadletName, "start", nil)
}

// quadletLifecycleAction POSTs /quadlets/{name}/{action} (with optional
// query params) and asserts 200 with the expected response shape.
func quadletLifecycleAction(t *testing.T, machineID, quadletName, action string, query url.Values) quadletLifecycleResponse {
	t.Helper()
	resp, raw := doQuadletLifecycle(t, machineID, quadletName, action, query)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"%s quadlet: expected 200, got %d. body: %s", action, resp.StatusCode, string(raw))

	var decoded quadletLifecycleResponse
	require.NoErrorf(t, json.Unmarshal(raw, &decoded),
		"%s quadlet: response must decode as QuadletLifecycleResponse. body: %s", action, string(raw))
	require.Equalf(t, quadletName, decoded.ID,
		"%s quadlet: response id must echo the quadlet name", action)
	require.Equalf(t, baseName(quadletName)+".service", decoded.Unit,
		"%s quadlet: response unit must be the derived systemd service unit", action)
	return decoded
}

// quadletLifecycleExpectError POSTs /quadlets/{name}/{action} and asserts
// the given HTTP status and error envelope code.
func quadletLifecycleExpectError(t *testing.T, machineID, quadletName, action string, query url.Values, wantStatus int, wantCode, msg string) {
	t.Helper()
	resp, raw := doQuadletLifecycle(t, machineID, quadletName, action, query)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"%s: expected %d, got %d. body: %s", msg, wantStatus, resp.StatusCode, string(raw))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", msg, string(raw))
	require.Equalf(t, wantCode, errResp.Error.Code,
		"%s: error code must be %s", msg, wantCode)
}

// doQuadletLifecycle issues the POST and returns the raw response/body.
func doQuadletLifecycle(t *testing.T, machineID, quadletName, action string, query url.Values) (*http.Response, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/" + action
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodPost, u, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoErrorf(t, err, "POST /quadlets/{id}/%s must connect to the in-process server", action)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

// baseName strips the trailing resource extension from a quadlet name
// ("it-x.container" → "it-x"), the base the systemd unit and the spawned
// container name are derived from.
func baseName(quadletName string) string {
	if idx := strings.LastIndex(quadletName, "."); idx > 0 {
		return quadletName[:idx]
	}
	return quadletName
}

// deleteQuadletWithQuery DELETEs /quadlets/{name} with optional query
// params and returns the raw status and body without asserting either —
// the DELETE subtests and cleanup build their own expectations on top.
func deleteQuadletWithQuery(t *testing.T, machineID, quadletName string, query url.Values) (int, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /quadlets/{id}")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// forceDeleteQuadlet DELETEs /quadlets/{name}?force=true and accepts
// 204 (removed, stops a running unit server-side) or 404 (already gone).
// Used by per-test cleanup only; the happy-path DELETE assertions live in
// the test body.
func forceDeleteQuadlet(t *testing.T, machineID, quadletName string) {
	t.Helper()
	status, raw := deleteQuadletWithQuery(t, machineID, quadletName,
		url.Values{"force": {"true"}})
	if status != http.StatusNoContent && status != http.StatusNotFound {
		t.Logf("cleanup delete quadlet %s: unexpected %d. body: %s (sweep is the backstop)",
			quadletName, status, string(raw))
	}
}

// removeSpawnedContainer force-removes the container the quadlet's
// systemd unit spawned (`systemd-<basename>` by podman quadlet naming).
// Best-effort: after a force delete the unit teardown usually removes it
// already, and the containers suite sweep covers anything left over.
func removeSpawnedContainer(t *testing.T, base string) {
	t.Helper()
	if out, err := podmanConnRun("rm", "-f", "systemd-"+base); err != nil {
		t.Logf("cleanup rm spawned container systemd-%s: %v (%s)", base, err, strings.TrimSpace(out))
	}
}

// ---------------------------------------------------------------------------
// State-transition polling
// ---------------------------------------------------------------------------

// quadletStatePollTimeout caps how long a state-transition poll waits.
// Generous against systemd status propagation lag and SSH round-trips,
// but bounded so a quadlet that never transitions fails instead of
// hanging the suite.
const quadletStatePollTimeout = 30 * time.Second

// quadletPollInterval is the delay between GET polls.
const quadletPollInterval = 500 * time.Millisecond

// waitForQuadletRunning polls GET /quadlets/{name} until state=="running"
// or the timeout elapses. GET's QuadletList-derived state is the source
// of truth (see slice header).
func waitForQuadletRunning(t *testing.T, machineID, quadletName, msg string) {
	t.Helper()
	deadline := time.Now().Add(quadletStatePollTimeout)
	var last string
	for time.Now().Before(deadline) {
		last = getQuadlet(t, machineID, quadletName).State
		if last == "running" {
			t.Logf("quadlet %s reached state=running", quadletName)
			return
		}
		time.Sleep(quadletPollInterval)
	}
	require.Equalf(t, "running", last,
		"%s: quadlet %s never reached state running within %s (last=%q)",
		msg, quadletName, quadletStatePollTimeout, last)
}

// waitForQuadletNotRunning polls GET /quadlets/{name} until the state
// leaves "running" or the timeout elapses, then asserts the terminal
// state and returns it.
//
// The terminal state of a stopped unit is "unknown", not "stopped":
// podman's QuadletList reports the systemd pair "inactive/dead" and
// parseStateFromStatus has no branch that matches it (dto.go:163-180).
// Waiting for a literal "stopped" would hang forever, so the observable
// contract here is "no longer running".
func waitForQuadletNotRunning(t *testing.T, machineID, quadletName, msg string) string {
	t.Helper()
	deadline := time.Now().Add(quadletStatePollTimeout)
	var last string
	for time.Now().Before(deadline) {
		last = getQuadlet(t, machineID, quadletName).State
		if last != "running" {
			t.Logf("quadlet %s left running (state=%q)", quadletName, last)
			return last
		}
		time.Sleep(quadletPollInterval)
	}
	require.Failf(t, msg,
		"quadlet %s never left state running within %s (last=%q)",
		quadletName, quadletStatePollTimeout, last)
	return last
}
