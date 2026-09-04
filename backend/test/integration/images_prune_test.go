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

// Test_Integration_Images_Prune is the image prune vertical slice
// (220-01) against POST /api/machines/{id}/images/prune. It seeds an
// unused image (pulled and tagged run-scoped, proving the registry
// path) plus an in-use image (busybox, held by a running container),
// prunes, and verifies the report entries and survival via the list
// endpoint:
//
//	POST /api/machines/{id}/images/prune → 200, entry per removed image
//
// Isolation model matches the images slice (080-02): run-scoped tag for
// assertions/cleanup, per-test t.Cleanup, and the shared suite sweep.
func Test_Integration_Images_Prune(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerImagesSweep()
	registerContainersSweep()

	machineID := suite.MachineID
	unusedRef := "localhost/it-" + suite.RunID + "-prune-alpine:latest"
	unusedNeedle := "it-" + suite.RunID + "-prune-alpine"
	containerName := "it-" + suite.RunID + "-prune-c"

	busyboxPreexisted := imageRepoPresent(t, machineID, "docker.io/library/busybox")
	alpinePreexisted := imageRepoPresent(t, machineID, "docker.io/library/alpine")

	// Fixtures: busybox (in use, must survive) and alpine (unused,
	// must be pruned). Pulls are idempotent test scaffolding over the
	// same podman connection the backend uses — NOT the system under
	// test (the API has no tag/pull-via-CLI path under test here).
	ensureBusyboxImage(t)
	out, err := podmanConnRun("run", "-d", "--name", containerName,
		"busybox:latest", "sh", "-c", "sleep 600")
	require.NoError(t, err, "fixture container create must succeed: %s", strings.TrimSpace(out))
	out, err = podmanConnRun("pull", "alpine:latest")
	require.NoError(t, err, "fixture alpine pull must succeed: %s", strings.TrimSpace(out))
	out, err = podmanConnRun("tag", "alpine:latest", unusedRef)
	require.NoError(t, err, "tag pulled image with run-scoped reference: %s", strings.TrimSpace(out))

	t.Cleanup(func() {
		if out, err := podmanConnRun("rm", "-f", containerName); err != nil {
			t.Logf("cleanup: rm %q best-effort failed: %v (%s)", containerName, err, strings.TrimSpace(out))
		}
		if out, err := podmanConnRun("rmi", unusedRef); err != nil {
			t.Logf("cleanup: rmi %q best-effort failed: %v (%s)", unusedRef, err, strings.TrimSpace(out))
		}
		if !alpinePreexisted {
			if out, err := podmanConnRun("rmi", "alpine:latest"); err != nil {
				t.Logf("cleanup: rmi alpine best-effort failed: %v (%s)", err, strings.TrimSpace(out))
			}
		}
		if !busyboxPreexisted {
			if out, err := podmanConnRun("rmi", "busybox:latest"); err != nil {
				t.Logf("cleanup: rmi busybox best-effort failed: %v (%s)", err, strings.TrimSpace(out))
			}
		}
	})

	// Capture both images' IDs before pruning so we can match them
	// against the prune report (after pruning the unused one is gone).
	unusedID := findRunScopedImageID(t, machineID, unusedNeedle)
	busyboxID := findRunScopedImageID(t, machineID, "docker.io/library/busybox")

	// --- AC: prune returns 200 with a report entry per removed image --
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+machineID+"/images/prune", "application/json", nil)
	require.NoError(t, err, "POST prune must connect to the in-process server")
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read prune response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"prune images: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var report []imagePruneDTO
	require.NoErrorf(t, json.Unmarshal(raw, &report),
		"prune response must decode as a report array. body: %s", string(raw))

	reported := map[string]bool{}
	for _, entry := range report {
		reported[entry.ID] = true
		assert.Empty(t, entry.Error, "prune must not report errors on a healthy machine")
	}

	// --- AC: the unused image is reported; the in-use image is not ----
	assert.True(t, reported[unusedID],
		"unused image %q must be reported as removed", unusedRef)
	assert.False(t, reported[busyboxID],
		"in-use image must not be reported as removed")

	// --- AC: the unused image is gone; the in-use image survives ------
	requireNoRunScopedImage(t, machineID, unusedNeedle)
	assert.True(t, imageRepoPresent(t, machineID, "docker.io/library/busybox"),
		"in-use image must survive the prune")
}

// imagePruneDTO mirrors one prune report entry.
type imagePruneDTO struct {
	ID    string `json:"id"`
	Size  uint64 `json:"size"`
	Error string `json:"error"`
}

// Test_Integration_Images_Prune_AccessControl pins the write gate on the
// prune route: read-level grantees and no-access users get 403.
func Test_Integration_Images_Prune_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/images/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/images/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})
}
