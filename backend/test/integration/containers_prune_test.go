//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Homerus25/podland/backend/internal/models"
)

// Test_Integration_Containers_Prune is the container prune vertical slice
// against POST /api/machines/{id}/containers/prune. It seeds two stopped
// containers plus one running container, prunes, and verifies the report
// entries and survival via the list endpoint:
//
//	POST /api/machines/{id}/containers/prune → 200, entry per removed container
//
// Isolation model matches the containers slice: run-scoped names for
// assertions/cleanup, per-test t.Cleanup, and the shared suite sweep.
func Test_Integration_Containers_Prune(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerContainersSweep()

	machineID := suite.MachineID

	stoppedA := "it-" + suite.RunID + "-prune-a"
	stoppedB := "it-" + suite.RunID + "-prune-b"
	running := "it-" + suite.RunID + "-prune-run"

	ensureBusyboxImage(t)
	for _, name := range []string{stoppedA, stoppedB} {
		// Detached like every other fixture: a foreground run would
		// block on the attach stream over the remote connection. The
		// one-shot command exits immediately, leaving the container in
		// the exited state the prune targets.
		out, err := podmanConnRun("run", "-d", "--name", name, "busybox:latest", "true")
		require.NoError(t, err, "fixture stopped container %q must start: %s", name, strings.TrimSpace(out))
	}
	out, err := podmanConnRun("run", "-d", "--name", running,
		"busybox:latest", "sh", "-c", "sleep 600")
	require.NoError(t, err, "fixture running container create must succeed: %s", strings.TrimSpace(out))

	t.Cleanup(func() {
		for _, name := range []string{stoppedA, stoppedB, running} {
			if out, err := podmanConnRun("rm", "-f", name); err != nil {
				t.Logf("cleanup: rm %q best-effort failed: %v (%s)", name, err, strings.TrimSpace(out))
			}
		}
	})

	// The prune report carries container IDs, not names, so the seeded
	// IDs are captured for the report assertions.
	containerID := func(name string) string {
		out, err := podmanConnRun("inspect", "--format", "{{.Id}}", name)
		require.NoError(t, err, "fixture container %q must be inspectable: %s", name, strings.TrimSpace(out))
		return strings.TrimSpace(out)
	}
	stoppedAID := containerID(stoppedA)
	stoppedBID := containerID(stoppedB)
	runningID := containerID(running)

	// --- AC: prune returns 200 with a report entry per removed container
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+machineID+"/containers/prune", "application/json", nil)
	require.NoError(t, err, "POST prune must connect to the in-process server")
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read prune response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"prune containers: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var report []containerPruneDTO
	require.NoErrorf(t, json.Unmarshal(raw, &report),
		"prune response must decode as a report array. body: %s", string(raw))

	reported := map[string]bool{}
	for _, entry := range report {
		reported[entry.ID] = true
		assert.Empty(t, entry.Error, "removal of seeded stopped container %q must not error", entry.ID)
	}
	assert.True(t, reported[stoppedAID], "stopped container %q must be reported as removed", stoppedA)
	assert.True(t, reported[stoppedBID], "stopped container %q must be reported as removed", stoppedB)
	assert.False(t, reported[runningID], "running container must not be reported as removed")

	// --- AC: stopped containers are gone; the running one survives ------
	names := listContainerNames(t, machineID)
	assert.NotContains(t, names, stoppedA, "pruned stopped container must be gone")
	assert.NotContains(t, names, stoppedB, "pruned stopped container must be gone")
	assert.Contains(t, names, running, "running container must survive the prune")
}

// containerPruneDTO mirrors one prune report entry.
type containerPruneDTO struct {
	ID    string `json:"id"`
	Size  uint64 `json:"size"`
	Error string `json:"error"`
}

// Test_Integration_Containers_Prune_AccessControl pins the write gate on
// the prune route: read-level grantees and no-access users get 403.
func Test_Integration_Containers_Prune_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/containers/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		client := noAccessUserClient(t)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/containers/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})
}
