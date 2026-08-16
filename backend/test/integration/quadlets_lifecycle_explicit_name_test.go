//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_Lifecycle_ExplicitName is the explicit-name
// variant of the quadlet lifecycle vertical slice (companion to
// Test_Integration_Quadlets_Lifecycle in quadlets_lifecycle_test.go).
// Same endpoints, same acceptance criteria:
//
//	POST /api/machines/{id}/quadlets/{name}/start   → 200, unit reaches running
//	POST /api/machines/{id}/quadlets/{name}/stop    → 200, unit leaves running
//	POST /api/machines/{id}/quadlets/{name}/restart → 200, unit back to running
//
// The one difference: the quadlet content pins `ContainerName=<explicit>`
// (podman's quadlet key for naming the managed container). Without it,
// podman's quadlet generator names the spawned container
// `systemd-<basename>`; with it, the container carries the explicit name
// verbatim. That naming assumption is baked into the backend in two
// places — resolveManagedContainerName (the logs path derives
// `systemd-<basename>` and cannot see an explicit name) and the
// integration suite's own removeSpawnedContainer cleanup helper — so this
// slice pins the invariant it must not break:
//
//   - The lifecycle endpoints address the systemd UNIT, which podman
//     derives from the quadlet FILE name (`<name>.container` →
//     `<name>.service`) — never from the container name inside the
//     content. Start/stop/restart must therefore work unchanged when the
//     content carries an explicit ContainerName.
//   - The spawned container is really named by the explicit name (and is
//     NOT the default `systemd-<basename>`) — asserted through the
//     containers list API while the unit runs.
//
// State verification policy, tripwires and isolation model are identical
// to the unnamed slice (see its header for the full reasoning). In
// particular the pinned gaps are expected to behave the same because they
// live on the DELETE/lifecycle service paths, which never look at the
// content's container name:
//
//   - DELETE on a running quadlet without `force` returns
//     QUADLET_RESOURCE_RUNNING (400) — Delete never wires the `stop` param.
//   - stop/restart with a `timeout` param fails 500 — the service maps
//     timeout to `systemctl --kill-seconds=<n>`, which is not a systemctl
//     option.
//
// Cleanup differs in exactly one way: the spawned container is removed by
// its explicit name (removeNamedContainer), not via the
// `systemd-<basename>` convention of removeSpawnedContainer.
func Test_Integration_Quadlets_Lifecycle_ExplicitName(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// Same transport preconditions as the unnamed lifecycle slice: the
	// actions shell out to `systemctl --user` over SSH as the machine's
	// SSH user, and quadlet start needs busybox pre-pulled.
	requireUserSystemd(t)
	ensureBusyboxImage(t)

	// Run-scoped names. The quadlet file on the remote machine becomes
	// `<quadletName>.container` and the systemd unit `<quadletName>.service`
	// (unchanged by the explicit container name). The spawned container is
	// named by the content's ContainerName — also run-scoped so an aborted
	// run leaves findable garbage for the sweep and can never collide with
	// another run's container (container names are unique per podman).
	quadletName := "it-" + suite.RunID + "-life-nm"
	explicitContainerName := "it-" + suite.RunID + "-life-nm-c"

	// Per-test cleanup, registered before create so it runs even if a
	// later phase fails. Best-effort; the sweep is the abort backstop.
	// The force delete stops a still-running unit server-side; the CLI
	// rm targets the EXPLICIT container name (this slice's whole point).
	t.Cleanup(func() {
		forceDeleteQuadlet(t, machineID, quadletName+".container")
		removeNamedContainer(t, explicitContainerName)
	})

	// --- Setup: create the runnable quadlet with an explicit name ------
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		Content:  explicitNameContainerQuadletContent(explicitContainerName),
	})

	// --- AC: lifecycle on a missing quadlet returns 404 ----------------
	t.Run("LifecycleOnMissingQuadletReturns404", func(t *testing.T) {
		missing := "it-" + suite.RunID + "-missing.container"
		for _, action := range []string{"start", "stop", "restart"} {
			quadletLifecycleExpectError(t, machineID, missing, action, nil,
				http.StatusNotFound, "QUADLET_NOT_FOUND",
				"lifecycle on a missing quadlet (explicit-name variant)")
		}
	})

	// --- AC: start transitions the quadlet to running ----------------
	t.Run("StartTransitionsToRunning", func(t *testing.T) {
		startQuadlet(t, machineID, quadletName+".container")
		waitForQuadletRunning(t, machineID, quadletName+".container",
			"start must transition the explicit-name quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC (variant-specific): the spawned container carries the
	// explicit name, and the default systemd-<basename> does not exist --
	//
	// This is the observable effect of `ContainerName=`: podman's quadlet
	// generator passes the explicit name to `podman run --name`, so the
	// container is listed under it while the unit runs, and no
	// `systemd-<basename>` container ever exists. The absence half is the
	// one that catches a backend/generator that quietly ignored the key.
	t.Run("SpawnedContainerCarriesExplicitName", func(t *testing.T) {
		requireContainerListed(t, machineID, explicitContainerName,
			"the quadlet-spawned container must be listed under the content's explicit ContainerName")
		requireContainerNotListed(t, machineID, "systemd-"+quadletName,
			"no default systemd-<basename> container may exist when the content pins ContainerName")
	})
	if t.Failed() {
		return
	}

	// --- AC: second start returns QUADLET_ALREADY_RUNNING (400) ------
	t.Run("SecondStartReturnsAlreadyRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, machineID, quadletName+".container", "start", nil,
			http.StatusBadRequest, "QUADLET_ALREADY_RUNNING",
			"second start on a running quadlet (explicit-name variant)")
	})
	if t.Failed() {
		return
	}

	// --- AC (gap tripwire): DELETE on a running quadlet without force
	// returns QUADLET_RESOURCE_RUNNING (400) --------------------------
	//
	// GAP (same as the unnamed slice): the DELETE `stop` query param is a
	// no-op — QuadletService.Delete ignores it, so deleting a running
	// quadlet without `force` is refused with 400. The explicit container
	// name does not change this: the refusal happens on the unit-state
	// check before any container naming is involved. If this assertion
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
	// GAP (same as the unnamed slice): LifecycleOptions.Timeout maps to
	// `systemctl --kill-seconds=<n>`, which is not a systemctl option on
	// any systemd version (it is a podman-run flag). systemctl exits 1
	// with "unrecognized option" and the API surfaces a 500. The gap is
	// in the systemctl argument builder, which never reads the quadlet
	// content, so the explicit container name cannot change the outcome.
	// If this assertion starts failing, the timeout mechanism was fixed —
	// replace it with the agreed working behavior.
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
			"restart must return the explicit-name quadlet to running")
	})
	if t.Failed() {
		return
	}

	// --- AC: stop transitions the quadlet out of running; force accepted
	t.Run("StopTransitionsOutOfRunning", func(t *testing.T) {
		quadletLifecycleAction(t, machineID, quadletName+".container", "stop",
			url.Values{"force": {"true"}})
		waitForQuadletNotRunning(t, machineID, quadletName+".container",
			"stop must transition the explicit-name quadlet out of running")
	})
	if t.Failed() {
		return
	}

	// --- AC: second stop returns QUADLET_NOT_RUNNING (400) ------------
	t.Run("SecondStopReturnsNotRunning", func(t *testing.T) {
		quadletLifecycleExpectError(t, machineID, quadletName+".container", "stop", nil,
			http.StatusBadRequest, "QUADLET_NOT_RUNNING",
			"second stop on a non-running quadlet (explicit-name variant)")
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
// Recipe + explicit-name cleanup helper
// ---------------------------------------------------------------------------

// explicitNameContainerQuadletContent builds the runnable recipe of the
// unnamed slice (busybox sleeping past the test's lifetime) with one
// addition: `ContainerName=<name>`, podman's quadlet key that names the
// managed container explicitly. Without the key, podman's generator
// names the container `systemd-<basename>`; with it, the container is
// named verbatim — the difference this slice exercises.
func explicitNameContainerQuadletContent(containerName string) string {
	return fmt.Sprintf(`[Container]
Image=busybox
Exec=sh -c "sleep 3600"
ContainerName=%s
`, containerName)
}

// removeNamedContainer force-removes the container by its explicit name
// over the podman connection — the explicit-name counterpart of the
// unnamed slice's removeSpawnedContainer (which targets the
// `systemd-<basename>` convention that ContainerName overrides).
// Best-effort: after a force delete the unit teardown usually removes it
// already, and the containers sweep covers anything left over.
func removeNamedContainer(t *testing.T, name string) {
	t.Helper()
	if out, err := podmanConnRun("rm", "-f", name); err != nil {
		t.Logf("cleanup rm explicit-name container %s: %v (%s)", name, err, strings.TrimSpace(out))
	}
}
