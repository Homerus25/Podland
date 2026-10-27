//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Networks_Prune is the networks prune vertical slice
// (spec #385) against POST /api/machines/{id}/networks/prune. It seeds
// two unused networks plus one in-use network (a running container is
// attached to it), prunes, and verifies the reports and survival via
// the list endpoint:
//
//	POST /api/machines/{id}/networks/prune → 200, entry per removed network
func Test_Integration_Networks_Prune(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerNetworksSweep()
	registerContainersSweep()

	machineID := suite.MachineID

	unusedA := "it-" + suite.RunID + "net-prune-a"
	unusedB := "it-" + suite.RunID + "net-prune-b"
	inUse := "it-" + suite.RunID + "net-prune-inuse"
	containerName := "it-" + suite.RunID + "net-prune-c"

	for _, name := range []string{unusedA, unusedB, inUse} {
		out, err := podmanConnRun("network", "create", name)
		require.NoError(t, err, "fixture network create %q must succeed: %s", name, strings.TrimSpace(out))
	}
	t.Cleanup(func() {
		_, _ = podmanConnRun("rm", "-f", containerName)
		for _, name := range []string{unusedA, unusedB, inUse} {
			_, _ = podmanConnRun("network", "rm", "-f", name)
		}
	})

	// Fixture: a container attached to the in-use network. Created, not
	// started: podman's prune skips any network with an attached
	// container regardless of the container's state, and a created
	// container pins that without depending on the nested container
	// runtime.
	ensureBusyboxImage(t)
	out, err := podmanConnRun("create", "--name", containerName,
		"--network", inUse, "busybox:latest", "true")
	require.NoError(t, err, "fixture container create must succeed: %s", strings.TrimSpace(out))

	// --- AC: prune returns 200 with a report entry per removed network
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+machineID+"/networks/prune", "application/json", nil)
	require.NoError(t, err, "POST prune must connect to the in-process server")
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read prune response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"prune networks: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var report []networkPruneDTO
	require.NoErrorf(t, json.Unmarshal(raw, &report),
		"prune response must decode as a report array. body: %s", string(raw))

	reported := map[string]bool{}
	for _, entry := range report {
		reported[entry.Name] = true
		assert.Empty(t, entry.Error, "removal of seeded unused network %q must not error", entry.Name)
	}
	assert.True(t, reported[unusedA], "unused network %q must be reported as removed", unusedA)
	assert.True(t, reported[unusedB], "unused network %q must be reported as removed", unusedB)
	assert.False(t, reported[inUse], "in-use network must not be reported as removed")

	// --- AC: unused networks are gone; the in-use network survives ----
	names := listNetworkNames(t, machineID)
	assert.NotContains(t, names, unusedA, "pruned unused network must be gone")
	assert.NotContains(t, names, unusedB, "pruned unused network must be gone")
	assert.Contains(t, names, inUse, "in-use network must survive the prune")
}

// networkPruneDTO mirrors one prune report entry.
type networkPruneDTO struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// listNetworkNames GETs the networks list and returns the names.
func listNetworkNames(t *testing.T, machineID string) []string {
	t.Helper()

	networks := listNetworks(t, machineID)
	names := make([]string, 0, len(networks))
	for _, network := range networks {
		names = append(names, network.Name)
	}
	return names
}

// Test_Integration_Networks_Prune_AccessControl pins the write gate on
// the prune route: read-level grantees and no-access users get 403.
func Test_Integration_Networks_Prune_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/networks/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		client := noAccessUserClient(t)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/networks/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})
}
