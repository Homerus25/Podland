//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Containers_Lifecycle is the containers vertical slice
// for the real-podman integration suite (PRD 080-03). It exercises the
// full container-management request flow end-to-end against the
// once-registered machine:
//
//	POST   /api/machines/{id}/containers             → 201 created
//	POST   /api/machines/{id}/containers/{cid}/start → running
//	GET    /api/machines/{id}/containers/{cid}       → state transitions
//	POST   /api/machines/{id}/containers/{cid}/stop  → no longer running
//	DELETE /api/machines/{id}/containers/{cid}       → 204 removed
//
// This proves create → start → stop → inspect → delete over the SSH
// tunnel, on top of the harness shared with the tracer and images slices.
//
// Isolation model (per the master PRD's "run-scoped tag + per-test
// cleanup + suite sweep" ruling):
//
//   - The container is created with a run-scoped name
//     (it-<runID>-lifecycle) so list/count assertions and cleanup can
//     find exactly what this run created. The container name is the
//     scoping handle because — unlike images — containers have no label
//     filter exposed by the list endpoint today.
//   - t.Cleanup force-removes the specific container created here, so a
//     failure between create and delete still leaves nothing behind.
//   - A suite-teardown sweep (registered once per process) removes any
//     run-scoped container left over from an aborted test (panic,
//     SIGINT before the per-test cleanup fired, …).
//
// Prerequisite: busybox must exist on the machine before ContainerCreate
// will succeed (podman does not auto-pull on create). The test ensures
// this itself rather than depending on the images slice having run —
// each test ensures its own prerequisites (PRD technical note).
func Test_Integration_Containers_Lifecycle(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerContainersSweep()

	machineID := suite.MachineID

	// --- AC #1: ensure busybox exists (idempotent) -------------------
	// Done over the same podman connection the backend uses, but via CLI
	// rather than the pull API: the API pull path is the images slice's
	// (080-02) concern, and using CLI here keeps this test focused on the
	// container API while remaining idempotent and cheap when cached.
	ensureBusyboxImage(t)

	// Run-scoped name so list/count assertions and the suite sweep can
	// find exactly what this run created.
	containerName := "it-" + suite.RunID + "-lifecycle"
	imageRef := "busybox:latest"
	// `sh -c "sleep 600"` keeps the container alive long enough to
	// observe the running state without racing an immediate exit (PRD
	// technical note: use a short-lived-but-observable command).
	createReq := containerCreateBody{
		Name:    containerName,
		Image:   imageRef,
		Command: []string{"sh", "-c", "sleep 600"},
	}

	// Per-test cleanup: force-remove the container we create. Registered
	// before create so it runs even if a later phase fails. Best-effort:
	// the sweep is the backstop for the abort case.
	var containerID string
	t.Cleanup(func() {
		if containerID == "" {
			return
		}
		if out, err := podmanConnRun("rm", "-f", containerID); err != nil {
			t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", containerID, err, strings.TrimSpace(out))
		}
	})

	// --- AC #2: POST creates a run-scoped-named container; 201 --------
	t.Run("CreateReturns201", func(t *testing.T) {
		containerID = createContainer(t, machineID, createReq)
		require.NotEmpty(t, containerID, "create must return a non-empty container id")
		t.Logf("container created: id=%s name=%s", containerID, containerName)
	})
	// Fail fast: every subsequent phase depends on a created container.
	// Returning here still runs t.Cleanup (registered above) and the
	// suite sweep — they are best-effort no-ops on a missing id.
	if t.Failed() {
		return
	}

	// --- AC #3: GET reflects created → running → exited transitions ---
	// Each phase is observed through GET (the inspect-backed source of
	// truth) rather than the start/stop response bodies, which hardcode
	// their state field (service.go returns literal "running"/"stopped")
	// and would mask a transition that didn't actually happen.
	t.Run("StartStopStateTransitions", func(t *testing.T) {
		// created: the state immediately after create, before start.
		waitForContainerState(t, machineID, containerID, "created",
			"container must be in created state before start")

		// running: after start.
		startContainer(t, machineID, containerID)
		waitForContainerState(t, machineID, containerID, "running",
			"container must reach running state after start")

		// exited: busybox `sleep` installs no SIGTERM handler, so podman's
		// default stop sequence SIGTERM-kills it (exit 143, non-zero) →
		// FromPodmanInspectContainer maps that to "exited". This is the
		// faithful reading of the spec's "stop → exits"; if a future
		// change makes the stop produce a clean exit (ExitCode==0, which
		// the DTO maps to "created"), we want that surfaced here rather
		// than masked by a looser "not running" assertion.
		stopContainer(t, machineID, containerID)
		waitForContainerState(t, machineID, containerID, "exited",
			"container must reach exited state after stop")
	})
	if t.Failed() {
		return
	}

	// --- AC #4: DELETE removes it; a subsequent list excludes it -------
	t.Run("DeleteRemovesContainer", func(t *testing.T) {
		// Prove presence first so a broken list endpoint (e.g. one that
		// always returns []) can't make the post-delete absence check pass
		// vacuously. The container is stopped here, so all=true is
		// required for it to appear (podman's list defaults to running).
		requireContainerListed(t, machineID, containerName,
			"stopped container must appear in the all=true list before delete")
		deleteContainer(t, machineID, containerID)
		// Null the id so the per-test cleanup doesn't try to remove a
		// container that's already gone (and would log a noisy error).
		containerID = ""
		requireContainerNotListed(t, machineID, containerName,
			"deleted container must not appear in the container list")
	})
}

// ---------------------------------------------------------------------------
// Prerequisite: ensure busybox exists on the machine (idempotent)
// ---------------------------------------------------------------------------

// ensureBusyboxImage guarantees busybox:latest is present on the machine,
// pulling it (cheap when cached) if it isn't. `podman create` does NOT
// auto-pull, so this must happen before the create call.
//
// Uses the podman CLI over the same connection the backend uses rather
// than the /images/pull API: the pull API is exercised end-to-end by the
// images slice (080-02), and re-exercising it here would couple this
// slice's pass/fail to the pull-stream path. `image exists` is a local,
// sub-second check; the pull is idempotent.
func ensureBusyboxImage(t *testing.T) {
	t.Helper()
	if _, err := podmanConnRun("image", "exists", "busybox:latest"); err == nil {
		t.Log("pre-check: busybox:latest already present on machine")
		return
	}
	t.Log("pre-check: busybox:latest missing, pulling (idempotent)")
	out, err := podmanConnRun("pull", "busybox:latest")
	require.NoError(t, err, "pull busybox:latest as a prerequisite: %s", strings.TrimSpace(out))
}

// ---------------------------------------------------------------------------
// HTTP helpers — each hits the in-process backend via the authenticated client
// ---------------------------------------------------------------------------

// containerCreateBody mirrors the JSON-accepted fields of
// containers.ContainerCreateRequest that this slice populates. Kept as a
// local type so the integration package doesn't import the containers
// domain package (matching the tracer/images slice style).
type containerCreateBody struct {
	Name    string   `json:"name,omitempty"`
	Image   string   `json:"image"`
	Command []string `json:"command,omitempty"`
}

// containerCreateResponse mirrors the fields of
// containers.ContainerCreateResponse that this slice consumes.
type containerCreateResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Image string `json:"image"`
}

// containerGetResponse mirrors the State/Name fields of
// containers.ContainerGetResponse (FromPodmanInspectContainer). Only the
// fields the state-transition assertions need are named.
type containerGetResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// createContainer POSTs /containers and returns the new container's ID.
// Asserts 201 (handler writes StatusCreated on success) and a non-empty id.
func createContainer(t *testing.T, machineID string, body containerCreateBody) string {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /containers must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"create container: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var created containerCreateResponse
	require.NoErrorf(t, json.Unmarshal(raw, &created),
		"create container: response must decode as ContainerCreateResponse. body: %s", string(raw))
	require.NotEmpty(t, created.ID, "create container: response missing id")
	require.Equal(t, body.Name, created.Name, "create container: response name must match request")
	require.Equal(t, "created", created.State, "create container: newly created container must report state=created")
	return created.ID
}

// startContainer POSTs /containers/{cid}/start and asserts 200.
func startContainer(t *testing.T, machineID, containerID string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/" + containerID + "/start"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "POST /containers/{id}/start")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"start container: expected 200, got %d. body: %s", resp.StatusCode, string(body))
}

// stopContainer POSTs /containers/{cid}/stop and asserts 200.
func stopContainer(t *testing.T, machineID, containerID string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/" + containerID + "/stop"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "POST /containers/{id}/stop")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"stop container: expected 200, got %d. body: %s", resp.StatusCode, string(body))
}

// deleteContainer DELETEs /containers/{cid} and asserts 204. By the time
// this runs the container has already been stopped, so no force is needed
// — the happy path mirrors what the AC describes (delete a stopped
// container). The per-test t.Cleanup keeps CLI `rm -f` as the abort
// backstop for a still-running container.
func deleteContainer(t *testing.T, machineID, containerID string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/" + containerID + "/"
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /containers/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode,
		"delete container: expected 204, got %d. body: %s", resp.StatusCode, string(body))
}

// getContainer GETs /containers/{cid} and returns the decoded response.
// Used by the state-transition pollers to observe the real (inspect-backed)
// state rather than the constant the start/stop handlers hardcode in their
// response bodies.
func getContainer(t *testing.T, machineID, containerID string) containerGetResponse {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/" + containerID + "/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /containers/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"get container: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got containerGetResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"get container: response must decode as ContainerGetResponse. body: %s", string(body))
	return got
}

// listContainerNames GETs /containers/?all=true and returns every
// container name on the machine. `all=true` is required because podman's
// list defaults to running-only — a stopped or created container would
// otherwise be invisible and break the "list excludes it" assertion.
func listContainerNames(t *testing.T, machineID string) []string {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/?all=true"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /containers/?all=true")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list containers: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	// ContainerListResult envelope: { containers: [...], total, timestamp }.
	var result struct {
		Containers []struct {
			Names []string `json:"names"`
		} `json:"containers"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &result),
		"list containers: response must decode as ContainerListResult. body: %s", string(body))

	var names []string
	for _, c := range result.Containers {
		names = append(names, c.Names...)
	}
	return names
}

// requireContainerListed asserts that name appears among the machine's
// containers. Proving presence before delete is what makes the post-delete
// absence check meaningful (otherwise a list that always returns [] would
// pass vacuously).
func requireContainerListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	require.Containsf(t, listContainerNames(t, machineID), name, "%s", msg)
}

// requireContainerNotListed asserts that name does NOT appear among the
// machine's containers — i.e. the delete actually removed it.
func requireContainerNotListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	require.NotContainsf(t, listContainerNames(t, machineID), name, "%s", msg)
}

// ---------------------------------------------------------------------------
// State-transition polling
// ---------------------------------------------------------------------------

// containerStatePollTimeout caps how long a state-transition poll waits.
// Generous against slow SSH round-trips through the tunnel, but bounded so
// a container that never reaches the target state fails the test instead
// of hanging the suite.
const containerStatePollTimeout = 30 * time.Second

// waitForContainerState polls GET /containers/{cid} until State==want or
// the timeout elapses. The inspect-backed GET is the source of truth for
// the real container state; the start/stop response bodies hardcode their
// state field and would mask a transition that didn't actually happen.
func waitForContainerState(t *testing.T, machineID, containerID, want string, msg string) {
	t.Helper()
	deadline := time.Now().Add(containerStatePollTimeout)
	var last containerGetResponse
	for time.Now().Before(deadline) {
		last = getContainer(t, machineID, containerID)
		if last.State == want {
			t.Logf("container %s reached state=%q", containerID, want)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.Equalf(t, want, last.State,
		"%s: container %s never reached state %q within %s (last=%q)",
		msg, containerID, want, containerStatePollTimeout, last.State)
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

// registerContainersSweepOnce ensures the containers suite-sweep cleanup
// is registered at most once per process. The sweep force-removes any
// container whose name mentions this run's RunID — i.e. garbage a test
// left behind when its own t.Cleanup didn't fire (panic, SIGINT before the
// per-test cleanup registered, …). Registered at the top of the
// containers test, so it is in place before any garbage is created.
//
// The sweep lists via `podman ps -a` (all states, not just running) so a
// created-but-never-started container is also swept. Best-effort: failures
// are logged by runCleanups, never fatal.
var registerContainersSweepOnce sync.Once

func registerContainersSweep() {
	registerContainersSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped containers", func() error {
			sweepRunScopedContainers(runID)
			return nil
		})
	})
}

// sweepRunScopedContainers lists every container on the machine (all
// states) and force-removes any whose name mentions the runID. See
// registerContainersSweep for the force/all-states rationale.
func sweepRunScopedContainers(runID string) {
	out, err := podmanConnRun("ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		// Best-effort: a failed list (e.g. machine already stopped) is
		// not a fatal teardown problem.
		return
	}
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.Contains(name, runID) {
			continue
		}
		// rm -f the run-scoped container. Ignore errors: the per-test
		// cleanup is the primary path; this is the backstop.
		_, _ = podmanConnRun("rm", "-f", name)
	}
}
