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
//	POST /api/hosts/{id}/quadlets/{name}/start   → 200, unit reaches running
//	POST /api/hosts/{id}/quadlets/{name}/stop    → 200, unit leaves running
//	POST /api/hosts/{id}/quadlets/{name}/restart → 200, unit back to running
//
// These endpoints run `systemctl <action> <unit>` over SSH via the
// sshexec service wired into app.New — scoped to the registered SSH
// user's manager (`--user`) unless that login is root, in which case the
// system manager owns the units — so this is the only slice that
// exercises the SSH-exec infrastructure against the real host.
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
// TRIPWIRES: none — the two 080-09 gaps this slice hosted (delete's
// unwired `stop` param, the broken timeout flag) are fixed; the slice
// now asserts the agreed behavior directly.
//
// Isolation/cleanup model (matching the CRUD slice): the quadlet is
// created with a run-scoped name (it-<runID>-life); t.Cleanup force-stops
// the unit, force-deletes the quadlet via the API and force-removes the
// systemd-spawned container over the podman connection. The suite sweep
// (registerQuadletsSweep) is the abort backstop.
func Test_Integration_Quadlets_Lifecycle(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	hostID := suite.HostID

	// The lifecycle actions shell out to `systemctl` over SSH as the
	// host's registered SSH user, so the host must (a) accept SSH
	// with the registered identity and (b) have a functioning systemd
	// manager for that user's scope — the user instance for a non-root
	// login, the system instance for root. If the target can't, skip
	// with a clear reason rather than failing (per the PRD).
	requireUserSystemd(t)

	// Quadlet start does not auto-pull; busybox must pre-exist.
	ensureBusyboxImage(t)

	// Run-scoped name: the file on the remote host becomes
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
		forceDeleteQuadlet(t, hostID, quadletName+".container")
		removeSpawnedContainer(t, quadletName)
	})

	// --- Setup: create the runnable quadlet --------------------------
	createQuadlet(t, hostID, quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})

	// The slow-stopping fixture: same busybox image, but the container's
	// PID 1 traps SIGTERM and StopTimeout=30 makes the stop job take ~30s —
	// long enough for a timeout=1 stop to give up waiting (504).
	slowQuadletName := "it-" + suite.RunID + "-slow"
	t.Cleanup(func() {
		forceDeleteQuadlet(t, hostID, slowQuadletName+".container")
		removeSpawnedContainer(t, slowQuadletName)
	})
	createQuadlet(t, hostID, quadletCreateBody{
		Name:     slowQuadletName,
		Resource: "container",
		Content: `[Container]
Image=busybox
Exec=sh -c "trap '' TERM; sleep 3600"
StopTimeout=30
`,
	})

	// --- AC: lifecycle on a missing quadlet returns 404 --------------
	t.Run("LifecycleOnMissingQuadletReturns404", func(t *testing.T) {
		missing := "it-" + suite.RunID + "-missing.container"
		for _, action := range []string{"start", "stop", "restart"} {
			quadletLifecycleExpectError(t, hostID, missing, action, nil,
				http.StatusNotFound, "QUADLET_NOT_FOUND",
				"lifecycle on a missing quadlet")
		}
	})

	// --- AC: start transitions the quadlet to running ----------------
	t.Run("StartTransitionsToRunning", func(t *testing.T) {
		startQuadlet(t, hostID, quadletName+".container")
		waitForQuadletRunning(t, hostID, quadletName+".container",
			"start must transition the quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC: second start returns QUADLET_ALREADY_RUNNING (400) ------
	t.Run("SecondStartReturnsAlreadyRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, hostID, quadletName+".container", "start", nil,
			http.StatusBadRequest, "QUADLET_ALREADY_RUNNING",
			"second start on a running quadlet")
	})
	if t.Failed() {
		return
	}

	// --- AC: stop of the slow-stopping fixture with timeout=1 gives up
	// at 504 ----------------------------------------------
	//
	// The stop runs as `timeout 1 systemctl --user stop <unit>`: the
	// wrapper expires (exit 124) long before the fixture's ~30s stop job
	// finishes and the API answers 504. The stop job itself keeps running
	// in systemd — the error says the WAIT was given up, not that the
	// stop failed. The envelope rewrites every 5xx body to
	// INTERNAL_ERROR with a generic message (pinned once in
	// internal/web), so the status line is the only wire-distinguishing
	// part; the QUADLET_STOP_TIMED_OUT sentinel is pinned at the service
	// seam (internal/domain/quadlets).
	t.Run("StopTimeoutExpiryReturns504", func(t *testing.T) {
		startQuadlet(t, hostID, slowQuadletName+".container")
		waitForQuadletRunning(t, hostID, slowQuadletName+".container",
			"the slow-stopping fixture must reach running before the timed stop")

		resp, raw := doQuadletLifecycle(t, hostID, slowQuadletName+".container", "stop",
			url.Values{"timeout": {"1"}})
		require.Equalf(t, http.StatusGatewayTimeout, resp.StatusCode,
			"slow-stopping fixture stopped with timeout=1: expected 504, got %d. body: %s",
			resp.StatusCode, string(raw))

		var errResp quadletErrorEnvelope
		require.NoErrorf(t, json.Unmarshal(raw, &errResp),
			"timed-out stop: response must decode as an error envelope. body: %s", string(raw))
	})
	if t.Failed() {
		return
	}

	// --- AC: a normal unit stopped with a timeout stops and answers 200
	t.Run("StopWithTimeoutReturns200", func(t *testing.T) {
		quadletLifecycleAction(t, hostID, quadletName+".container", "stop",
			url.Values{"timeout": {"30"}})
		waitForQuadletNotRunning(t, hostID, quadletName+".container",
			"a normal-unit stop with a timeout param must still transition out of running")
	})
	if t.Failed() {
		return
	}

	// --- AC: restart returns the quadlet to running --------------------
	t.Run("RestartReturnsToRunning", func(t *testing.T) {
		// The timeout stop above left the unit stopped; restart preconditions
		// on running, so bring it back first.
		startQuadlet(t, hostID, quadletName+".container")
		waitForQuadletRunning(t, hostID, quadletName+".container",
			"re-start after the timeout stop must reach running")

		restarted := quadletLifecycleAction(t, hostID, quadletName+".container", "restart", nil)
		require.NotEmpty(t, restarted.RestartedAt,
			"restart response must include restartedAt")
		waitForQuadletRunning(t, hostID, quadletName+".container",
			"restart must return the quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC: stop transitions the quadlet out of running ----------------
	t.Run("StopTransitionsOutOfRunning", func(t *testing.T) {
		quadletLifecycleAction(t, hostID, quadletName+".container", "stop", nil)
		waitForQuadletNotRunning(t, hostID, quadletName+".container",
			"stop must transition the quadlet out of running")
	})
	if t.Failed() {
		return
	}

	// --- AC: second stop returns QUADLET_NOT_RUNNING (400) ------------
	t.Run("SecondStopReturnsNotRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, hostID, quadletName+".container", "stop", nil,
			http.StatusBadRequest, "QUADLET_NOT_RUNNING",
			"second stop on a non-running quadlet")
	})
	if t.Failed() {
		return
	}

	// --- AC: DELETE on a running quadlet with stop=false is refused ---
	// Re-start first so the refusal is doing real work (a stopped quadlet
	// deletes without force regardless of stop).
	t.Run("DeleteOnRunningWithStopFalseReturns400", func(t *testing.T) {
		startQuadlet(t, hostID, quadletName+".container")
		waitForQuadletRunning(t, hostID, quadletName+".container",
			"re-start before the stop=false delete must reach running")

		status, raw := deleteQuadletWithQuery(t, hostID, quadletName+".container",
			url.Values{"stop": {"false"}})
		require.Equalf(t, http.StatusBadRequest, status,
			"delete running quadlet with stop=false: expected 400, got %d. body: %s",
			status, string(raw))

		var errResp quadletErrorEnvelope
		require.NoErrorf(t, json.Unmarshal(raw, &errResp),
			"delete running quadlet (stop=false): error response must decode. body: %s", string(raw))
		require.Equal(t, "QUADLET_RESOURCE_RUNNING", errResp.Error.Code,
			"delete running quadlet (stop=false): error code must be QUADLET_RESOURCE_RUNNING")
	})
	if t.Failed() {
		return
	}

	// --- AC: DELETE on a running quadlet with the default stop stops
	// through the seam then removes (204) ------------------------------
	t.Run("DeleteOnRunningWithDefaultStopReturns204", func(t *testing.T) {
		status, raw := deleteQuadletWithQuery(t, hostID, quadletName+".container", nil)
		require.Equalf(t, http.StatusNoContent, status,
			"delete running quadlet with default stop: expected 204, got %d. body: %s",
			status, string(raw))

		requireQuadletNotListed(t, hostID, quadletName+".container",
			"stop-then-remove must take the quadlet out of the quadlet list")
	})
	if t.Failed() {
		return
	}

	// --- AC: DELETE on a running quadlet with force=true returns 204 --
	//
	// The default-stop delete above already consumed the quadlet, so
	// re-create it and bring it back to running first — the force must
	// remove over a genuinely running resource.
	t.Run("DeleteOnRunningWithForceReturns204", func(t *testing.T) {
		createQuadlet(t, hostID, quadletCreateBody{
			Name:     quadletName,
			Resource: "container",
			Content:  runnableContainerQuadletContent,
		})
		startQuadlet(t, hostID, quadletName+".container")
		waitForQuadletRunning(t, hostID, quadletName+".container",
			"re-start before force delete must reach running")

		status, raw := deleteQuadletWithQuery(t, hostID, quadletName+".container",
			url.Values{"force": {"true"}})
		require.Equalf(t, http.StatusNoContent, status,
			"delete running quadlet with force: expected 204, got %d. body: %s",
			status, string(raw))

		requireQuadletNotListed(t, hostID, quadletName+".container",
			"force-deleted quadlet must not appear in the quadlet list")
	})
}

// ---------------------------------------------------------------------------
// Prerequisite probe: user systemd over SSH
// ---------------------------------------------------------------------------

// requireUserSystemd skips the calling test unless the host's user
// systemd answers over SSH — the transport every lifecycle action uses.
// The probe dials with the exact coordinates the backend registered the
// host with (suite.goodHostRegistration), so what it probes is
// what the sshexec service will actually hit. Skips (never fails) when
// the target can't support user systemd units, per the PRD.
func requireUserSystemd(t *testing.T) {
	t.Helper()
	if !userSystemdAvailable() {
		t.Skip("host user systemd unusable (is-system-running did not answer running/degraded)")
	}
}

// userSystemdAvailable is the boolean probe behind requireUserSystemd:
// it dials the shared good host's SSH target (see
// userSystemdAvailableOn) and checks whether its user systemd answers.
func userSystemdAvailable() bool {
	return userSystemdAvailableOn(suite.goodHostRegistration)
}

// userSystemdAvailableOn probes an arbitrary target the same way: it
// dials the target's SSH endpoint with the registered identity and
// checks `systemctl is-system-running` answers with "running" or
// "degraded" — proof that SSH works and a systemd manager answers for
// the registered user (readable system-manager queries serve any login;
// the backend's own scope choice rides on the registered SSH user). The quadlet
// API surface needs this on the host (podman's quadlet list/install/
// remove dial the user systemd over DBus), so callers gate or skip
// quadlet coverage on the result — e.g. on the systemd-free container
// test host, or on the second test host via
// SecondHost.Target. Returns false for an unusable or unreachable
// target.
func userSystemdAvailableOn(target reachableTarget) bool {
	key, err := ssh.ParsePrivateKey([]byte(target.SSHKey))
	if err != nil {
		return false
	}

	cfg := &ssh.ClientConfig{
		User:            target.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // test fixture; matches sshexec dialer
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(target.Port)), cfg)
	if err != nil {
		return false
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return false
	}
	defer session.Close()

	out, _ := session.CombinedOutput("systemctl is-system-running")
	status := strings.TrimSpace(string(out))
	// "degraded" still means the user manager is up; individual units may
	// have failed but systemctl itself works, which is all we need. Note
	// systemctl exits 1 for "degraded", so the exit error alone must not
	// fail the probe.
	return status == "running" || status == "degraded"
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
func startQuadlet(t *testing.T, hostID, quadletName string) quadletLifecycleResponse {
	t.Helper()
	return quadletLifecycleAction(t, hostID, quadletName, "start", nil)
}

// quadletLifecycleAction POSTs /quadlets/{name}/{action} (with optional
// query params) and asserts 200 with the expected response shape.
func quadletLifecycleAction(t *testing.T, hostID, quadletName, action string, query url.Values) quadletLifecycleResponse {
	t.Helper()
	resp, raw := doQuadletLifecycle(t, hostID, quadletName, action, query)
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
func quadletLifecycleExpectError(t *testing.T, hostID, quadletName, action string, query url.Values, wantStatus int, wantCode, msg string) {
	t.Helper()
	resp, raw := doQuadletLifecycle(t, hostID, quadletName, action, query)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"%s: expected %d, got %d. body: %s", msg, wantStatus, resp.StatusCode, string(raw))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", msg, string(raw))
	require.Equalf(t, wantCode, errResp.Error.Code,
		"%s: error code must be %s", msg, wantCode)
}

// doQuadletLifecycle issues the POST and returns the raw response/body.
func doQuadletLifecycle(t *testing.T, hostID, quadletName, action string, query url.Values) (*http.Response, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/hosts/" + hostID + "/quadlets/" + quadletName + "/" + action
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
func deleteQuadletWithQuery(t *testing.T, hostID, quadletName string, query url.Values) (int, []byte) {
	t.Helper()
	u := suite.ServerURL + "/api/hosts/" + hostID + "/quadlets/" + quadletName + "/"
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
func forceDeleteQuadlet(t *testing.T, hostID, quadletName string) {
	t.Helper()
	status, raw := deleteQuadletWithQuery(t, hostID, quadletName,
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
func waitForQuadletRunning(t *testing.T, hostID, quadletName, msg string) {
	t.Helper()
	deadline := time.Now().Add(quadletStatePollTimeout)
	var last string
	for time.Now().Before(deadline) {
		last = getQuadlet(t, hostID, quadletName).State
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
func waitForQuadletNotRunning(t *testing.T, hostID, quadletName, msg string) string {
	t.Helper()
	deadline := time.Now().Add(quadletStatePollTimeout)
	var last string
	for time.Now().Before(deadline) {
		last = getQuadlet(t, hostID, quadletName).State
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
