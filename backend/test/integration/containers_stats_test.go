//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// containerStatsEntry mirrors the numeric fields of
// containers.ContainerStatsResponse that this slice asserts on. Kept as a
// local type so the integration package doesn't import the containers
// domain package (matching the lifecycle slice style).
type containerStatsEntry struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	CPUPercent      float64 `json:"cpuPercent"`
	MemoryUsage     uint64  `json:"memoryUsage"`
	MemoryLimit     uint64  `json:"memoryLimit"`
	MemoryPercent   float64 `json:"memoryPercent"`
	NetworkBytesIn  uint64  `json:"networkBytesIn"`
	NetworkBytesOut uint64  `json:"networkBytesOut"`
	BlockBytesIn    uint64  `json:"blockBytesIn"`
	BlockBytesOut   uint64  `json:"blockBytesOut"`
	PIDs            uint64  `json:"pids"`
}

// Test_Integration_Containers_Stats pins both container stats routes'
// happy paths against the container test host (spec 415, ticket 416):
//
//	GET /api/hosts/{id}/containers/stats            → 200 list envelope
//	GET /api/hosts/{id}/containers/{cid}/stats      → 200 single DTO
//
// A run-scoped busybox container is started first — podman samples stats
// only for running containers, and the server-side sampling takes ~1s per
// request, which the HTTP call simply waits out.
//
// Isolation model: same as the lifecycle slice — run-scoped name
// (it-<runID>-stats), per-test t.Cleanup force-remove, and the suite
// sweep registered at the top as the abort backstop.
func Test_Integration_Containers_Stats(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerContainersSweep()

	hostID := suite.HostID
	ensureBusyboxImage(t)

	containerName := "it-" + suite.RunID + "-stats"
	createReq := containerCreateBody{
		Name:    containerName,
		Image:   "busybox:latest",
		Command: []string{"sh", "-c", "sleep 600"},
	}

	var containerID string
	t.Cleanup(func() {
		if containerID == "" {
			return
		}
		if out, err := podmanConnRun("rm", "-f", containerID); err != nil {
			t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", containerID, err, strings.TrimSpace(out))
		}
	})

	// --- Setup: create and start the run-scoped container ---------------
	containerID = createContainer(t, hostID, createReq)
	startContainer(t, hostID, containerID)
	waitForContainerState(t, hostID, containerID, "running",
		"container must be running before stats can be sampled")

	// --- AC: the all-stats route answers the list envelope --------------
	t.Run("AllStatsSnapshot", func(t *testing.T) {
		url := suite.ServerURL + "/api/hosts/" + hostID + "/containers/stats"
		resp, err := suite.AuthenticatedClient().Get(url)
		require.NoError(t, err, "GET /containers/stats must connect to the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"all-stats: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

		// ContainerStatsResult envelope: { containers: [...], total, timestamp }.
		var result struct {
			Containers []containerStatsEntry `json:"containers"`
			Total      int                   `json:"total"`
			Timestamp  string                `json:"timestamp"`
		}
		require.NoErrorf(t, json.Unmarshal(raw, &result),
			"all-stats: response must decode as the stats list envelope. body: %s", string(raw))
		require.NotEmpty(t, result.Timestamp, "all-stats: the envelope must carry a timestamp")

		// The started container's entry must be present with raw numeric
		// fields. Other containers may exist on the host, so presence is
		// asserted, not exclusivity.
		var found *containerStatsEntry
		for i := range result.Containers {
			if result.Containers[i].ID == containerID {
				found = &result.Containers[i]
			}
		}
		require.NotNilf(t, found, "all-stats: the running container's entry must be present. body: %s", string(raw))
		require.Equal(t, containerName, found.Name, "all-stats: the entry must carry the container name")
		require.Greater(t, found.MemoryLimit, uint64(0), "all-stats: memory limit must be a raw numeric value")
		require.GreaterOrEqual(t, found.PIDs, uint64(1), "all-stats: a running container reports at least one process")
	})

	// --- AC: the per-container route answers the single DTO -------------
	t.Run("PerContainerStats", func(t *testing.T) {
		url := suite.ServerURL + "/api/hosts/" + hostID + "/containers/" + containerID + "/stats"
		resp, err := suite.AuthenticatedClient().Get(url)
		require.NoError(t, err, "GET /containers/{cid}/stats must connect to the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"per-container stats: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

		var got containerStatsEntry
		require.NoErrorf(t, json.Unmarshal(raw, &got),
			"per-container stats: response must decode as the stats DTO. body: %s", string(raw))
		require.Equal(t, containerID, got.ID, "per-container stats: the DTO must carry the container ID")
		require.Equal(t, containerName, got.Name, "per-container stats: the DTO must carry the container name")
		require.Greater(t, got.MemoryLimit, uint64(0), "per-container stats: memory limit must be a raw numeric value")
		require.GreaterOrEqual(t, got.PIDs, uint64(1), "per-container stats: a running container reports at least one process")
	})
}
