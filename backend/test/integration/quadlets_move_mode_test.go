//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy transfer slice, move mode (issue 211). POST
// /api/machines/{id}/quadlets/{quadletId}/move with
// {"targetMachineId": ..., "mode": "move"} installs the source quadlet's
// file on the target and removes it from the source only after that install
// succeeded (target manager reload included). The removal honors the delete
// flow's preconditions: a RUNNING source quadlet refuses the move with 400
// QUADLET_RESOURCE_RUNNING unless stop or force is set, and stop performs a
// stop-then-move.
//
// The partial envelope (install kept, source removal failed) is pinned at
// the service seam — a removal failure cannot be simulated through the real
// podman surface without breaking the machine. Like the copy slice this
// gates on the user-systemd probe and needs the second, independent test
// host from the harness (issue 208).

// moveQuadletModeBody is the JSON body of a move-mode transfer with
// optional flag pairs (stop/force).
func moveQuadletModeBody(t *testing.T, targetMachineID string, flags map[string]bool) string {
	t.Helper()
	payload := map[string]any{"targetMachineId": targetMachineID, "mode": "move"}
	for name, value := range flags {
		payload[name] = value
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(body)
}

// moveQuadletMode POSTs a move-mode transfer as the suite user and returns
// the decoded envelope, asserting the 200 shape.
func moveQuadletMode(t *testing.T, sourceMachineID, quadletName, targetMachineID string, flags map[string]bool) quadletMoveResponse {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(moveURL(sourceMachineID, quadletName),
		"application/json", bytes.NewReader([]byte(moveQuadletModeBody(t, targetMachineID, flags))))
	require.NoError(t, err, "POST /quadlets/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"move quadlet: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got quadletMoveResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"move quadlet: response must decode as QuadletMoveResponse. body: %s", string(body))
	return got
}

// Test_Integration_Quadlets_MoveMode pins the move contract end to end: a
// moved quadlet lands on machine B and disappears from machine A, a RUNNING
// source quadlet without stop/force is refused with nothing installed, and
// stop=true performs the stop-then-move.
func Test_Integration_Quadlets_MoveMode(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — the move transfer needs independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("machine B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	ensureBusyboxImage(t)

	machineA := suite.MachineID
	base := "it-" + suite.RunID + "-mvmove"
	runQuadlet := base + "-run"

	// Per-test cleanup, registered before the creates; the suite sweeps are
	// the abort backstop. Both machines are cleaned: a move lands on B and
	// deletes from A, and every fixture starts on A.
	for _, machine := range []string{machineA, b.MachineID} {
		t.Cleanup(func() { forceDeleteQuadlet(t, machine, base+".container") })
		t.Cleanup(func() { forceDeleteQuadlet(t, machine, runQuadlet+".container") })
	}
	t.Cleanup(func() { removeSpawnedContainer(t, runQuadlet) })

	createQuadlet(t, machineA, quadletCreateBody{
		Name:     base,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})

	// --- AC: move removes the source quadlet only after the target
	// install succeeded --------------------------------------------------
	t.Run("MoveInstallsOnTargetAndRemovesSource", func(t *testing.T) {
		got := moveQuadletMode(t, machineA, base+".container", b.MachineID, nil)

		assert.Equal(t, base+".container", got.ID)
		assert.Equal(t, "container", got.Resource)
		assert.Equal(t, "moved", got.Status)
		assert.Empty(t, got.Error)
		assert.Equal(t, machineA, got.SourceMachineID)
		assert.Equal(t, b.MachineID, got.TargetMachineID)

		onB := getQuadlet(t, b.MachineID, base+".container")
		assert.Equal(t, runnableContainerQuadletContent, onB.Content,
			"the target file must carry the source's content verbatim")
		assert.NotEqual(t, "running", onB.State,
			"the transferred unit is installed but never started")

		getQuadletExpectNotFound(t, machineA, base+".container")
	})

	// --- AC: a running source quadlet without stop/force fails the delete
	// precondition and installs nothing -----------------------------------
	t.Run("RunningSourceWithoutStopOrForceRefused", func(t *testing.T) {
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     runQuadlet,
			Resource: "container",
			Content:  runnableContainerQuadletContent,
		})
		startQuadlet(t, machineA, runQuadlet+".container")
		waitForQuadletRunning(t, machineA, runQuadlet+".container",
			"the source quadlet must reach running before the refused move")

		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			moveURL(machineA, runQuadlet+".container"),
			moveQuadletModeBody(t, b.MachineID, nil),
			http.StatusBadRequest, "QUADLET_RESOURCE_RUNNING")

		assert.Empty(t, listQuadletsFiltered(t, b.MachineID,
			url.Values{"name": {runQuadlet + ".container"}}),
			"a refused move must install nothing on the target")
		waitForQuadletRunning(t, machineA, runQuadlet+".container",
			"a refused move must leave the source quadlet running")
	})

	// --- AC: stop performs a stop-then-move ------------------------------
	t.Run("StopPerformsStopThenMove", func(t *testing.T) {
		got := moveQuadletMode(t, machineA, runQuadlet+".container", b.MachineID,
			map[string]bool{"stop": true})

		assert.Equal(t, "moved", got.Status)
		assert.Empty(t, got.Error)

		onB := getQuadlet(t, b.MachineID, runQuadlet+".container")
		assert.Equal(t, runnableContainerQuadletContent, onB.Content)
		assert.NotEqual(t, "running", onB.State,
			"the transferred unit is installed but never started")

		getQuadletExpectNotFound(t, machineA, runQuadlet+".container")
	})

	// --- AC: force passes through to removal like the delete flow --------
	t.Run("ForceMovesOverARunningSource", func(t *testing.T) {
		// The stop subtest consumed the running fixture; bring it back up
		// for the forced variant.
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     runQuadlet,
			Resource: "container",
			Content:  runnableContainerQuadletContent,
		})
		startQuadlet(t, machineA, runQuadlet+".container")
		waitForQuadletRunning(t, machineA, runQuadlet+".container",
			"the source quadlet must reach running before the forced move")

		got := moveQuadletMode(t, machineA, runQuadlet+".container", b.MachineID,
			map[string]bool{"force": true})

		assert.Equal(t, "moved", got.Status)
		assert.Empty(t, got.Error)

		onB := getQuadlet(t, b.MachineID, runQuadlet+".container")
		assert.Equal(t, runnableContainerQuadletContent, onB.Content)
		assert.NotEqual(t, "running", onB.State,
			"the transferred unit is installed but never started")

		getQuadletExpectNotFound(t, machineA, runQuadlet+".container")
	})
}
