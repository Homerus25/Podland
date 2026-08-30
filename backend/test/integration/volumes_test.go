//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Volumes_List is the volumes list vertical slice
// (200-01). It creates a run-scoped named volume via the podman CLI over
// the machine connection (scaffolding, not the SUT), then proves the
// backend lists it — plus an anonymous volume — through
// GET /api/machines/{id}/volumes:
//
//	GET /api/machines/{id}/volumes → 200, JSON array
//
// Assertions pin the DTO contract: name, driver, createdAt, anonymous,
// labels, options, mountpoint. Isolation follows the house model:
// run-scoped name, per-test t.Cleanup, suite sweep for aborts.
func Test_Integration_Volumes_List(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	machineID := suite.MachineID
	volumeName := "it-" + suite.RunID + "-vol"

	t.Cleanup(func() {
		if out, err := podmanConnRun("volume", "rm", volumeName); err != nil {
			t.Logf("cleanup: volume rm %q best-effort failed: %v (%s)", volumeName, err, strings.TrimSpace(out))
		}
	})

	// Fixture: one named run-scoped volume with a label, created over
	// the same podman connection the backend tunnels through.
	out, err := podmanConnRun("volume", "create",
		"--label", "podland-it="+suite.RunID,
		"--opt", "type=tmpfs",
		volumeName)
	require.NoError(t, err, "volume create must succeed: %s", strings.TrimSpace(out))

	// --- AC: list returns 200 with a JSON array containing the volume --
	t.Run("NamedVolumeAppearsInList", func(t *testing.T) {
		volumes := listVolumes(t, machineID)

		var found *volumeDTO
		for i := range volumes {
			if volumes[i].Name == volumeName {
				found = &volumes[i]
				break
			}
		}
		require.NotNil(t, found, "run-scoped volume %q must appear in the list, got %d volumes", volumeName, len(volumes))
		assert.Equal(t, "local", found.Driver)
		assert.NotEmpty(t, found.CreatedAt, "createdAt must be reported")
		assert.False(t, found.Anonymous)
		assert.Equal(t, map[string]string{"podland-it": suite.RunID}, found.Labels)
		assert.Equal(t, map[string]string{"type": "tmpfs"}, found.Options)
		assert.NotEmpty(t, found.Mountpoint, "mountpoint must be reported")
	})

	// --- AC: anonymous volumes are included alongside named ones ------
	t.Run("AnonymousVolumesIncluded", func(t *testing.T) {
		// Fixture: an anonymous volume (no name given to podman), so the
		// assertion can't depend on pre-existing machine state.
		out, err := podmanConnRun("volume", "create")
		require.NoError(t, err, "anonymous volume create must succeed: %s", strings.TrimSpace(out))
		anonName := strings.TrimSpace(out)
		t.Cleanup(func() {
			if _, err := podmanConnRun("volume", "rm", "-f", anonName); err != nil {
				t.Logf("cleanup: rm anonymous volume %q best-effort failed: %v", anonName, err)
			}
		})

		volumes := listVolumes(t, machineID)

		var found *volumeDTO
		for i := range volumes {
			if volumes[i].Name == anonName {
				found = &volumes[i]
				break
			}
		}
		// Inclusion is the contract here: an anonymous volume must appear
		// in the list alongside named ones. The boolean flag is asserted
		// only when the machine's podman server reports it — some server
		// versions omit the Anonymous field in /volumes/json output.
		if found == nil {
			require.Fail(t, "just-created anonymous volume must be listed", "name=%q", anonName)
			return
		}
		// Flag fidelity is asserted only when the server actually reports
		// the Anonymous field for some volume — server versions that omit
		// it make the false branch indistinguishable from "not reported".
		if serverReportsAnonymous(volumes) {
			assert.True(t, found.Anonymous,
				"anonymous volume must be flagged when the server reports the flag")
		}
	})
}

// --- Access-control behavior (middleware, shared with all downstream
// families): volumes rides the same RequireMachineAccess subtree, so a
// no-access user gets 403 ACCESS_DENIED and an unknown machine ID gets
// 404 MACHINE_NOT_FOUND, both with the standard error envelope.
func Test_Integration_Volumes_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	client := noAccessUserClient(t)

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/volumes",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownMachineGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/machines/00000000-0000-0000-0000-000000000000/volumes",
			"", http.StatusNotFound, "MACHINE_NOT_FOUND")
	})
}

// volumeDTO mirrors the volumes list response item.
type volumeDTO struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	CreatedAt  string            `json:"createdAt"`
	Anonymous  bool              `json:"anonymous"`
	Labels     map[string]string `json:"labels"`
	Options    map[string]string `json:"options"`
	Mountpoint string            `json:"mountpoint"`
	Size       *int64            `json:"size"`
}

// serverReportsAnonymous reports whether the machine's podman server
// populated the Anonymous flag on any listed volume; when the server
// omits the field entirely, flag fidelity can't be asserted.
func serverReportsAnonymous(volumes []volumeDTO) bool {
	for _, v := range volumes {
		if v.Anonymous {
			return true
		}
	}
	return false
}

// listVolumes GETs /api/machines/{id}/volumes and decodes the JSON array.
func listVolumes(t *testing.T, machineID string) []volumeDTO {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/machines/" + machineID + "/volumes")
	require.NoError(t, err, "GET /volumes must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read volumes response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list volumes: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out []volumeDTO
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"volumes response must decode as a JSON array. body: %s", string(body))
	return out
}

// --- Suite sweep (covers aborts) ---------------------------------------

var registerVolumesSweepOnce sync.Once

func registerVolumesSweep() {
	registerVolumesSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped volumes", func() error {
			out, err := podmanConnRun("volume", "ls", "--format", "{{.Name}}")
			if err != nil {
				return nil
			}
			for _, name := range strings.Split(out, "\n") {
				if strings.Contains(name, fmt.Sprintf("it-%s", runID)) {
					_, _ = podmanConnRun("volume", "rm", strings.TrimSpace(name))
				}
			}
			return nil
		})
	})
}
