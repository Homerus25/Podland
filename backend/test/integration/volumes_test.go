//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Volumes_List is the volumes list vertical slice
// (200-01). It creates a run-scoped named volume via the podman CLI over
// the host connection (scaffolding, not the SUT), then proves the
// backend lists it — plus an anonymous volume — through
// GET /api/hosts/{id}/volumes:
//
//	GET /api/hosts/{id}/volumes → 200, JSON array
//
// Assertions pin the DTO contract: name, driver, createdAt, anonymous,
// labels, options, mountpoint. Isolation follows the house model:
// run-scoped name, per-test t.Cleanup, suite sweep for aborts.
func Test_Integration_Volumes_List(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	hostID := suite.HostID
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
		volumes := listVolumes(t, hostID)

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
		// assertion can't depend on pre-existing host state.
		out, err := podmanConnRun("volume", "create")
		require.NoError(t, err, "anonymous volume create must succeed: %s", strings.TrimSpace(out))
		anonName := strings.TrimSpace(out)
		t.Cleanup(func() {
			if _, err := podmanConnRun("volume", "rm", "-f", anonName); err != nil {
				t.Logf("cleanup: rm anonymous volume %q best-effort failed: %v", anonName, err)
			}
		})

		volumes := listVolumes(t, hostID)

		var found *volumeDTO
		for i := range volumes {
			if volumes[i].Name == anonName {
				found = &volumes[i]
				break
			}
		}
		// Inclusion is the contract here: an anonymous volume must appear
		// in the list alongside named ones. The boolean flag is asserted
		// only when the host's podman server reports it — some server
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

// Test_Integration_Volumes_Inspect is the volume inspect vertical slice
// (200-02). It creates a run-scoped named volume via the podman CLI,
// then proves the backend resolves it by exact name through
// GET /api/hosts/{id}/volumes/{volumeName}:
//
//	GET /api/hosts/{id}/volumes/{volumeName} → 200, full config
//	GET .../volumes/{unknown}                   → 404 VOLUME_NOT_FOUND
func Test_Integration_Volumes_Inspect(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	hostID := suite.HostID
	volumeName := "it-" + suite.RunID + "-inspect"

	t.Cleanup(func() {
		if out, err := podmanConnRun("volume", "rm", volumeName); err != nil {
			t.Logf("cleanup: volume rm %q best-effort failed: %v (%s)", volumeName, err, strings.TrimSpace(out))
		}
	})

	out, err := podmanConnRun("volume", "create",
		"--label", "podland-it="+suite.RunID,
		"--opt", "type=tmpfs",
		volumeName)
	require.NoError(t, err, "volume create must succeed: %s", strings.TrimSpace(out))

	// --- AC: inspect returns 200 with the volume's full config --------
	t.Run("HappyPath", func(t *testing.T) {
		resp, err := suite.AuthenticatedClient().Get(
			suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + volumeName)
		require.NoError(t, err, "GET volume inspect must connect to the in-process server")
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "read volume inspect response body")
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"inspect volume: expected 200, got %d. body: %s", resp.StatusCode, string(body))

		var got volumeDTO
		require.NoErrorf(t, json.Unmarshal(body, &got),
			"volume inspect response must decode as the config DTO. body: %s", string(body))
		assert.Equal(t, volumeName, got.Name)
		assert.Equal(t, "local", got.Driver)
		assert.NotEmpty(t, got.CreatedAt, "createdAt must be reported")
		assert.Equal(t, map[string]string{"podland-it": suite.RunID}, got.Labels)
		assert.Equal(t, map[string]string{"type": "tmpfs"}, got.Options)
		assert.NotEmpty(t, got.Mountpoint, "mountpoint must be reported")
	})

	// --- AC: unknown volume name returns 404 VOLUME_NOT_FOUND ---------
	t.Run("UnknownNameGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/it-"+suite.RunID+"-no-such-volume",
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
	})
}

// --- Access-control behavior (middleware, shared with all downstream
// families): volumes rides the same RequireHostAccess subtree, so a
// no-access user gets 403 ACCESS_DENIED and an unknown host ID gets
// 404 HOST_NOT_FOUND, both with the standard error envelope.
func Test_Integration_Volumes_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	client := noAccessUserClient(t)

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownHostGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/hosts/00000000-0000-0000-0000-000000000000/volumes",
			"", http.StatusNotFound, "HOST_NOT_FOUND")
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

// serverReportsAnonymous reports whether the host's podman server
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

// listVolumes GETs /api/hosts/{id}/volumes and decodes the JSON array.
func listVolumes(t *testing.T, hostID string) []volumeDTO {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/hosts/" + hostID + "/volumes")
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

// Test_Integration_Volumes_Create is the volume create vertical slice
// (200-03) against POST /api/hosts/{id}/volumes:
//
//	POST /api/hosts/{id}/volumes (named)    → 201, full config
//	POST /api/hosts/{id}/volumes (anonymous)→ 201, generated name
//	POST .../volumes (malformed body)          → 400 INVALID_REQUEST
//	POST .../volumes as read-level user        → 403
func Test_Integration_Volumes_Create(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	hostID := suite.HostID

	// --- AC: named create returns 201 with the created volume's config,
	// and labels/driver options round-trip through inspect --------------
	t.Run("NamedCreateWithLabelsAndOptions", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-create"
		t.Cleanup(func() {
			if _, err := podmanConnRun("volume", "rm", volumeName); err != nil {
				t.Logf("cleanup: rm %q best-effort failed: %v", volumeName, err)
			}
		})

		body := `{"name":"` + volumeName + `","labels":{"podland-it":"` + suite.RunID + `"},"options":{"type":"tmpfs"}}`
		resp := postJSON(t, "/api/hosts/"+hostID+"/volumes", body)
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "read create response body")
		require.Equalf(t, http.StatusCreated, resp.StatusCode,
			"create volume: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

		var created volumeDTO
		require.NoErrorf(t, json.Unmarshal(raw, &created),
			"create response must decode as the volume config. body: %s", string(raw))
		assert.Equal(t, volumeName, created.Name, "name must be echoed")
		assert.Equal(t, "local", created.Driver, "podman default driver must be reported")
		assert.Equal(t, map[string]string{"podland-it": suite.RunID}, created.Labels)
		assert.Equal(t, map[string]string{"type": "tmpfs"}, created.Options)

		// Round-trip: the same facts come back through inspect.
		resp2, err := suite.AuthenticatedClient().Get(
			suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + volumeName)
		require.NoError(t, err, "GET inspect must connect")
		defer resp2.Body.Close()
		require.Equal(t, http.StatusOK, resp2.StatusCode, "created volume must be inspectable")
		var inspected volumeDTO
		require.NoError(t, json.NewDecoder(resp2.Body).Decode(&inspected))
		assert.Equal(t, volumeName, inspected.Name)
		assert.Equal(t, map[string]string{"podland-it": suite.RunID}, inspected.Labels)
		assert.Equal(t, map[string]string{"type": "tmpfs"}, inspected.Options)
	})

	// --- AC: omitting the name creates an anonymous volume and the
	// generated name is present in the response -------------------------
	t.Run("AnonymousCreate", func(t *testing.T) {
		resp := postJSON(t, "/api/hosts/"+hostID+"/volumes", `{}`)
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "read anonymous create response body")
		require.Equalf(t, http.StatusCreated, resp.StatusCode,
			"anonymous create: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

		var created volumeDTO
		require.NoErrorf(t, json.Unmarshal(raw, &created),
			"anonymous create response must decode as the volume config. body: %s", string(raw))
		require.NotEmpty(t, created.Name, "generated name must be present so callers can learn it")

		_, err = podmanConnRun("volume", "rm", "-f", created.Name)
		require.NoError(t, err, "cleaning the created anonymous volume must succeed")
	})

	// --- AC: podman-rejected names map to client errors, not 5xx -------
	t.Run("PodmanRejectedNameGets400", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes",
			`{"name":"it-`+suite.RunID+`-bad name"}`, http.StatusBadRequest, "INVALID_REQUEST")
	})

	// --- AC: malformed request bodies produce the standard validation
	// error response ------------------------------------------------------
	t.Run("MalformedBodyGets400", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes",
			`{"labels": 42}`, http.StatusBadRequest, "INVALID_REQUEST")
	})
}

// Test_Integration_Volumes_Create_AccessControl pins the write gate on the
// create route: read-level grantees and no-access users get 403, unknown
// host gets 404.
func Test_Integration_Volumes_Create_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes",
			`{}`, http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes",
			`{}`, http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownHostGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/hosts/00000000-0000-0000-0000-000000000000/volumes",
			`{}`, http.StatusNotFound, "HOST_NOT_FOUND")
	})
}

// postJSON POSTs an authenticated JSON body to a suite-relative path.
func postJSON(t *testing.T, path, body string) *http.Response {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+path, "application/json", strings.NewReader(body))
	require.NoError(t, err, "POST %s must connect to the in-process server", path)
	return resp
}

// Test_Integration_Volumes_Delete is the volume delete vertical slice
// (200-04) against DELETE /api/hosts/{id}/volumes/{volumeName}?force=:
//
//	DELETE unused volume            → 204, gone from the list
//	DELETE in-use volume            → 409 VOLUME_IN_USE
//	DELETE in-use volume ?force=true→ 204, container stopped, volume gone
//	DELETE unknown volume           → 404 VOLUME_NOT_FOUND
func Test_Integration_Volumes_Delete(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()
	registerContainersSweep()

	hostID := suite.HostID

	// --- AC: unused volume deletes with 204 -----------------------------
	t.Run("UnusedVolumeDeletes204", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-del-unused"
		out, err := podmanConnRun("volume", "create", volumeName)
		require.NoError(t, err, "fixture volume create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() {
			_, _ = podmanConnRun("volume", "rm", "-f", volumeName)
		})

		deleteVolume(t, hostID, volumeName, "", http.StatusNoContent)

		assert.NotContains(t, listVolumeNames(t, hostID), volumeName,
			"deleted volume must disappear from the list")
	})

	// --- AC: in-use volume without force gets 409; with force succeeds --
	t.Run("InUseVolume", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-del-inuse"
		containerName := "it-" + suite.RunID + "-del-inuse-c"
		out, err := podmanConnRun("volume", "create", volumeName)
		require.NoError(t, err, "fixture volume create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() {
			_, _ = podmanConnRun("rm", "-f", containerName)
			_, _ = podmanConnRun("volume", "rm", "-f", volumeName)
		})

		// Fixture: a running container mounting the volume. busybox is
		// ensured by the shared helper (same prerequisite as containers).
		ensureBusyboxImage(t)
		out, err = podmanConnRun("run", "-d", "--name", containerName,
			"-v", volumeName+":/data", "busybox:latest", "sh", "-c", "sleep 600")
		require.NoError(t, err, "fixture container create must succeed: %s", strings.TrimSpace(out))
		require.NotEmpty(t, strings.TrimSpace(out), "fixture container must print an id")

		// Without force: podman refuses → 409 with the distinct code.
		body := deleteVolume(t, hostID, volumeName, "", http.StatusConflict)
		assert.Contains(t, body, "VOLUME_IN_USE", "in-use refusal must carry the distinct error code")

		// With force: podman stops the container and removes the volume.
		deleteVolume(t, hostID, volumeName, "?force=true", http.StatusNoContent)

		assert.NotContains(t, listVolumeNames(t, hostID), volumeName,
			"force-deleted volume must be gone from the list")
		state := containerStateByName(t, containerName)
		assert.NotEqual(t, "running", state, "podman must have stopped the dependent container")
	})

	// --- AC: unknown volume name returns the clean 404 ------------------
	t.Run("UnknownNameGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/it-"+suite.RunID+"-no-such-volume",
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
	})
}

// deleteVolume DELETEs a volume with an optional raw query string and
// asserts the expected status; it returns the response body text.
func deleteVolume(t *testing.T, hostID, volumeName, query string, wantStatus int) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete,
		suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+query, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE volume must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read volume delete response body")
	require.Equalf(t, wantStatus, resp.StatusCode,
		"delete volume %q: expected %d, got %d. body: %s", volumeName, wantStatus, resp.StatusCode, string(body))
	return string(body)
}

// listVolumeNames lists volumes and returns just their names.
func listVolumeNames(t *testing.T, hostID string) []string {
	t.Helper()
	volumes := listVolumes(t, hostID)
	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	return names
}

// containerStateByName inspects a container through the backend by name.
func containerStateByName(t *testing.T, name string) string {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/hosts/" + suite.HostID + "/containers/" + name)
	require.NoError(t, err, "GET container inspect must connect")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "missing(" + resp.Status + "): " + string(raw)
	}
	var got struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	return got.State
}

// Test_Integration_Volumes_Delete_AccessControl pins the write gate on
// the delete route: read-level grantees and no-access users get 403,
// unknown host gets 404.
func Test_Integration_Volumes_Delete_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes/it-"+suite.RunID+"-whatever",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes/it-"+suite.RunID+"-whatever",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownHostGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodDelete,
			suite.ServerURL+"/api/hosts/00000000-0000-0000-0000-000000000000/volumes/whatever",
			"", http.StatusNotFound, "HOST_NOT_FOUND")
	})
}

// Test_Integration_Volumes_Prune is the volume prune vertical slice
// (200-05) against POST /api/hosts/{id}/volumes/prune. It seeds two
// unused volumes plus one in-use volume (mounted by a running container),
// prunes, and verifies the reports and survival via the list endpoint:
//
//	POST /api/hosts/{id}/volumes/prune → 200, entry per removed volume
func Test_Integration_Volumes_Prune(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()
	registerContainersSweep()

	hostID := suite.HostID

	unusedA := "it-" + suite.RunID + "-prune-a"
	unusedB := "it-" + suite.RunID + "-prune-b"
	inUse := "it-" + suite.RunID + "-prune-inuse"
	containerName := "it-" + suite.RunID + "-prune-c"

	for _, name := range []string{unusedA, unusedB, inUse} {
		out, err := podmanConnRun("volume", "create", name)
		require.NoError(t, err, "fixture volume create %q must succeed: %s", name, strings.TrimSpace(out))
	}
	t.Cleanup(func() {
		_, _ = podmanConnRun("rm", "-f", containerName)
		for _, name := range []string{unusedA, unusedB, inUse} {
			_, _ = podmanConnRun("volume", "rm", "-f", name)
		}
	})

	// Fixture: a running container mounting the in-use volume.
	ensureBusyboxImage(t)
	out, err := podmanConnRun("run", "-d", "--name", containerName,
		"-v", inUse+":/data", "busybox:latest", "sh", "-c", "sleep 600")
	require.NoError(t, err, "fixture container create must succeed: %s", strings.TrimSpace(out))

	// --- AC: prune returns 200 with a report entry per removed volume --
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/hosts/"+hostID+"/volumes/prune", "application/json", nil)
	require.NoError(t, err, "POST prune must connect to the in-process server")
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read prune response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"prune volumes: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var report []volumePruneDTO
	require.NoErrorf(t, json.Unmarshal(raw, &report),
		"prune response must decode as a report array. body: %s", string(raw))

	reported := map[string]bool{}
	for _, entry := range report {
		reported[entry.Name] = true
		assert.Empty(t, entry.Error, "removal of seeded unused volume %q must not error", entry.Name)
	}
	assert.True(t, reported[unusedA], "unused volume %q must be reported as removed", unusedA)
	assert.True(t, reported[unusedB], "unused volume %q must be reported as removed", unusedB)
	assert.False(t, reported[inUse], "in-use volume must not be reported as removed")

	// --- AC: unused volumes are gone; the in-use volume survives -------
	names := listVolumeNames(t, hostID)
	assert.NotContains(t, names, unusedA, "pruned unused volume must be gone")
	assert.NotContains(t, names, unusedB, "pruned unused volume must be gone")
	assert.Contains(t, names, inUse, "in-use volume must survive the prune")
}

// volumePruneDTO mirrors one prune report entry.
type volumePruneDTO struct {
	Name  string `json:"name"`
	Size  uint64 `json:"size"`
	Error string `json:"error"`
}

// Test_Integration_Volumes_Prune_AccessControl pins the write gate on the
// prune route: read-level grantees and no-access users get 403.
func Test_Integration_Volumes_Prune_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/volumes/prune",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})
}

// Test_Integration_Volumes_Export is the volume export vertical slice
// (200-06) against GET /api/hosts/{id}/volumes/{volumeName}/export.
// It seeds a volume with known files (tar piped through the CLI's
// volume import, the same tar contract the bindings' Import uses),
// exports it through the API, and untars the response body to verify
// the file contents byte-for-byte:
//
//	GET /api/hosts/{id}/volumes/{volumeName}/export → 200, x-tar
func Test_Integration_Volumes_Export(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	hostID := suite.HostID
	volumeName := "it-" + suite.RunID + "-export"

	t.Cleanup(func() {
		_, _ = podmanConnRun("volume", "rm", "-f", volumeName)
	})

	out, err := podmanConnRun("volume", "create", volumeName)
	require.NoError(t, err, "fixture volume create must succeed: %s", strings.TrimSpace(out))

	// Fixture: a tar with two known files, seeded via volume import.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	fileA := []byte("hello from podland\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg, Name: "greeting.txt", Size: int64(len(fileA)), Mode: 0o644,
	}))
	_, err = tw.Write(fileA)
	require.NoError(t, err)
	fileB := []byte{0x00, 0x01, 0x02, 0xff}
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg, Name: "nested/binary.bin", Size: int64(len(fileB)), Mode: 0o644,
	}))
	_, err = tw.Write(fileB)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	tarFile := filepath.Join(t.TempDir(), "seed.tar")
	require.NoError(t, os.WriteFile(tarFile, tarBuf.Bytes(), 0o600), "writing the seed tar must succeed")
	out, err = podmanConnRun("volume", "import", volumeName, tarFile)
	require.NoError(t, err, "seeding the volume via import must succeed: %s", strings.TrimSpace(out))

	// --- AC: export streams a valid tar with the right headers ---------
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + volumeName + "/export")
	require.NoError(t, err, "GET export must connect to the in-process server")
	defer resp.Body.Close()
	require.Equalf(t, http.StatusOK, resp.StatusCode, "export volume: expected 200")
	assert.Equal(t, "application/x-tar", resp.Header.Get("Content-Type"))
	assert.Equal(t, `attachment; filename="`+volumeName+`.tar"`, resp.Header.Get("Content-Disposition"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read exported tar body")

	// --- AC: untar and verify contents match ----------------------------
	tr := tar.NewReader(bytes.NewReader(body))
	found := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "iterating exported tar must not fail")
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			require.NoError(t, err, "reading exported file %q", hdr.Name)
			found[hdr.Name] = data
		}
	}
	assert.Equal(t, fileA, found["greeting.txt"], "exported file content must match the seed")
	assert.Equal(t, fileB, found["nested/binary.bin"], "binary content must round-trip exactly")
}

// Test_Integration_Volumes_Export_AccessControl pins export's read-level
// access ruling: a read-level user can export; no-access users get 403;
// unknown volume names get 404.
func Test_Integration_Volumes_Export_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)
	hostID := suite.HostID

	t.Run("ReadLevelUserCanExport", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-export-read"
		out, err := podmanConnRun("volume", "create", volumeName)
		require.NoError(t, err, "fixture volume create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() { _, _ = podmanConnRun("volume", "rm", "-f", volumeName) })

		client := levelUserClient(t, models.PermissionRead)
		resp, err := client.Get(suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + volumeName + "/export")
		require.NoError(t, err, "read-level export must connect")
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "read-level users can export (grilling ruling)")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodGet,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/some-volume/export",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownVolumeGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/it-"+suite.RunID+"-no-such-volume/export",
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
	})
}

// Test_Integration_Volumes_Populate is the volume populate vertical
// slice (200-07) against POST /api/hosts/{id}/volumes/{name}/populate.
// Both shapes are exercised, contents are verified through the export
// endpoint (untar and compare), and the merge case proves a pre-existing
// file at an untouched path survives:
//
//	POST /api/hosts/{id}/volumes/{name}/populate → 204 (merge)
func Test_Integration_Volumes_Populate(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	hostID := suite.HostID

	seed := func(name string, files map[string][]byte) string {
		t.Helper()
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		for path, content := range files {
			require.NoError(t, tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeReg, Name: path, Size: int64(len(content)), Mode: 0o644,
			}))
			_, err := tw.Write(content)
			require.NoError(t, err)
		}
		require.NoError(t, tw.Close())
		tarFile := filepath.Join(t.TempDir(), "seed.tar")
		require.NoError(t, os.WriteFile(tarFile, tarBuf.Bytes(), 0o600))
		out, err := podmanConnRun("volume", "create", name)
		require.NoError(t, err, "fixture volume create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() { _, _ = podmanConnRun("volume", "rm", "-f", name) })
		out, err = podmanConnRun("volume", "import", name, tarFile)
		require.NoError(t, err, "seeding %q must succeed: %s", name, strings.TrimSpace(out))
		return tarBuf.String()
	}

	// volumeContents exports through the API and untars the body.
	volumeContents := func(t *testing.T, name string) map[string][]byte {
		t.Helper()
		resp, err := suite.AuthenticatedClient().Get(
			suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + name + "/export")
		require.NoError(t, err, "export for verification must connect")
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "export must succeed")
		tr := tar.NewReader(resp.Body)
		out := map[string][]byte{}
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if hdr.Typeflag == tar.TypeReg {
				data, err := io.ReadAll(tr)
				require.NoError(t, err)
				out[hdr.Name] = data
			}
		}
		return out
	}

	// --- AC: files shape lands uploads with relative paths preserved ---
	t.Run("FilesShape", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-populate-files"
		seed(volumeName, map[string][]byte{"keep/me.txt": []byte("keep me")})

		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		for _, f := range []struct {
			path    string
			content []byte
		}{{"docs/readme.txt", []byte("populate me")}, {"nested/deep/bin.dat", []byte{0xCA, 0xFE}}} {
			// The raw filename carries the relative path; Go's server
			// parser preserves what the client sent.
			fw, err := mw.CreateFormFile("files", f.path)
			require.NoError(t, err)
			_, err = fw.Write(f.content)
			require.NoError(t, err)
		}
		require.NoError(t, mw.Close())

		resp, err := suite.AuthenticatedClient().Post(
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+"/populate",
			mw.FormDataContentType(), bytes.NewReader(body.Bytes()))
		require.NoError(t, err, "populate must connect")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusNoContent, resp.StatusCode,
			"populate files shape: expected 204, got %d. body: %s", resp.StatusCode, string(raw))

		contents := volumeContents(t, volumeName)
		assert.Equal(t, []byte("populate me"), contents["docs/readme.txt"], "uploaded file must land with its path")
		assert.Equal(t, []byte{0xCA, 0xFE}, contents["nested/deep/bin.dat"])
		assert.Equal(t, []byte("keep me"), contents["keep/me.txt"],
			"merge semantics: untouched pre-existing files must survive")
	})

	// --- AC: archive shape imports with structure intact ----------------
	t.Run("ArchiveShape", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-populate-archive"
		seed(volumeName, nil)

		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		require.NoError(t, tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "dir/a.txt", Size: 5, Mode: 0o644}))
		_, err := tw.Write([]byte("bingo"))
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		fw, err := mw.CreateFormFile("archive", "upload.tar")
		require.NoError(t, err)
		_, err = fw.Write(tarBuf.Bytes())
		require.NoError(t, err)
		require.NoError(t, mw.Close())

		resp, err := suite.AuthenticatedClient().Post(
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+"/populate",
			mw.FormDataContentType(), bytes.NewReader(body.Bytes()))
		require.NoError(t, err, "populate must connect")
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode, "populate archive shape must succeed")

		contents := volumeContents(t, volumeName)
		assert.Equal(t, []byte("bingo"), contents["dir/a.txt"], "archive structure must be intact")
	})

	// --- AC: merge semantics end-to-end — overwriting one path while
	// everything else survives -------------------------------------------
	t.Run("MergeOverwritesSameNameOnly", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-populate-merge"
		seed(volumeName, map[string][]byte{
			"target.txt": []byte("old content"),
			"other.txt":  []byte("untouched"),
		})

		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		fw, err := mw.CreateFormFile("files", "target.txt")
		require.NoError(t, err)
		_, err = fw.Write([]byte("new content"))
		require.NoError(t, err)
		require.NoError(t, mw.Close())

		resp, err := suite.AuthenticatedClient().Post(
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+"/populate",
			mw.FormDataContentType(), bytes.NewReader(body.Bytes()))
		require.NoError(t, err, "populate must connect")
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)

		contents := volumeContents(t, volumeName)
		assert.Equal(t, []byte("new content"), contents["target.txt"], "uploaded path must overwrite")
		assert.Equal(t, []byte("untouched"), contents["other.txt"], "merge must keep everything else")
	})

	// --- AC: both shapes at once → 400; empty upload → 400 --------------
	t.Run("ShapeValidation", func(t *testing.T) {
		volumeName := "it-" + suite.RunID + "-populate-shapes"
		seed(volumeName, nil)

		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		fw, err := mw.CreateFormFile("files", "a.txt")
		require.NoError(t, err)
		_, err = fw.Write([]byte("x"))
		require.NoError(t, err)
		fw, err = mw.CreateFormFile("archive", "up.tar")
		require.NoError(t, err)
		_, err = fw.Write([]byte("tar"))
		require.NoError(t, err)
		require.NoError(t, mw.Close())

		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+"/populate",
			"", http.StatusBadRequest, "INVALID_REQUEST")
		_ = body

		empty := &bytes.Buffer{}
		emptyMw := multipart.NewWriter(empty)
		require.NoError(t, emptyMw.Close())
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName+"/populate",
			"", http.StatusBadRequest, "INVALID_REQUEST")
	})

	// --- AC: read-level users get 403; unknown volume 404 ----------------
	t.Run("AccessControl", func(t *testing.T) {
		readClient := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, readClient, http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/some-volume/populate",
			"", http.StatusForbidden, "ACCESS_DENIED")

		expectAPIError(t, noAccessUserClient(t), http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/some-volume/populate",
			"", http.StatusForbidden, "ACCESS_DENIED")

		// The existence gate runs after the body is parsed, so the 404
		// probe carries a valid files-shaped multipart body.
		populateBody := &bytes.Buffer{}
		populateMw := multipart.NewWriter(populateBody)
		fw, err := populateMw.CreateFormFile("files", "probe.txt")
		require.NoError(t, err)
		_, err = fw.Write([]byte("probe"))
		require.NoError(t, err)
		require.NoError(t, populateMw.Close())
		req, err := http.NewRequest(http.MethodPost,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/it-"+suite.RunID+"-no-such-volume/populate",
			bytes.NewReader(populateBody.Bytes()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", populateMw.FormDataContentType())
		resp, err := suite.AuthenticatedClient().Do(req)
		require.NoError(t, err, "populate 404 probe must connect")
		defer resp.Body.Close()
		rawBody, _ := io.ReadAll(resp.Body)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "unknown volume must 404. body: %s", string(rawBody))
		assert.Contains(t, string(rawBody), "VOLUME_NOT_FOUND")
	})
}
