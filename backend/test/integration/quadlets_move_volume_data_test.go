//go:build integration

package integration

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy volume DATA migration slice (issue 214). With
// migrateVolumeData set, the transfer resolves every volume-kind `Volume=`
// reference, refuses non-empty target volumes (409) unless
// overwriteVolumes merges, exports the source volume and imports it into
// the target, and reports per-volume outcomes both in the NDJSON event
// stream and the envelope's volumeMigration block.
//
// Everything here is API-driven on both hosts: volumes are created
// (POST /volumes), seeded (multipart /populate) and verified (untarred
// /export) through the in-process server, so the slice runs wherever the
// two-host harness runs. The user-systemd probe gates the quadlet
// surface, as in every quadlet slice.

// moveNDJSONEvent mirrors one NDJSON event of a migrating transfer; only
// the fields this slice asserts are named.
type moveNDJSONEvent struct {
	Type         string `json:"type"`
	Phase        string `json:"phase"`
	Ref          string `json:"ref"`
	PodmanVolume string `json:"podmanVolume"`
	Reason       string `json:"reason"`
	Error        *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Result *struct {
		Status          string `json:"status"`
		SourceHostID    string `json:"sourceHostId"`
		TargetHostID    string `json:"targetHostId"`
		VolumeMigration *struct {
			Volumes []struct {
				Ref          string `json:"ref"`
				PodmanVolume string `json:"podmanVolume"`
				Outcome      string `json:"outcome"`
				Reason       string `json:"reason"`
			} `json:"volumes"`
		} `json:"volumeMigration"`
		SecretTransfer *struct {
			Secrets []struct {
				Name    string `json:"name"`
				Outcome string `json:"outcome"`
				Reason  string `json:"reason"`
			} `json:"secrets"`
		} `json:"secretTransfer"`
	} `json:"result"`
}

// parseMoveEvents decodes the streamed NDJSON body into events.
func parseMoveEvents(t *testing.T, body string) []moveNDJSONEvent {
	t.Helper()
	var events []moveNDJSONEvent
	scanner := bufio.NewScanner(bytes.NewReader([]byte(body)))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event moveNDJSONEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must decode: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// postMoveMigrate posts a migrating transfer and returns the NDJSON body
// (the stream answers 200 with the failure riding the terminal event).
func postMoveMigrate(t *testing.T, sourceHostID, quadletName, body string) []moveNDJSONEvent {
	t.Helper()
	status, raw := postMove(t, sourceHostID, quadletName, body)
	require.Equalf(t, http.StatusOK, status,
		"the migrating transfer's stream itself must succeed. body: %s", string(raw))
	return parseMoveEvents(t, string(raw))
}

// migratingMoveBody builds a migrating transfer body with the given mode and
// extra flags.
func migratingMoveBody(t *testing.T, targetHostID, mode string, extra map[string]any) string {
	t.Helper()
	if extra == nil {
		extra = map[string]any{}
	}
	extra["migrateVolumeData"] = true
	return moveCompanionBody(t, targetHostID, mode, extra)
}

// createVolumeViaAPI creates a volume through the write-level create
// endpoint and registers its API deletion as cleanup.
func createVolumeViaAPI(t *testing.T, hostID, volumeName string) {
	t.Helper()
	resp := postJSON(t, "/api/hosts/"+hostID+"/volumes", `{"name":"`+volumeName+`"}`)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"volume create %s: expected 201, got %d. body: %s", volumeName, resp.StatusCode, string(raw))
	t.Cleanup(func() {
		del, err := suite.AuthenticatedClient().Do(standardRequest(t, http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+hostID+"/volumes/"+volumeName))
		if err != nil {
			t.Logf("cleanup volume %s: %v", volumeName, err)
			return
		}
		del.Body.Close()
	})
}

// standardRequest builds an authenticated request with the given method.
func standardRequest(t *testing.T, method, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	return req
}

// seedVolumeViaAPI writes files into an existing volume through the
// multipart populate endpoint (files shape).
func seedVolumeViaAPI(t *testing.T, hostID, volumeName string, files map[string][]byte) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for path, content := range files {
		fw, err := mw.CreateFormFile("files", path)
		require.NoError(t, err)
		_, err = fw.Write(content)
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
		"populate %s: expected 204, got %d. body: %s", volumeName, resp.StatusCode, string(raw))
}

// exportVolumeViaAPI exports a host's volume through the API and
// untars the body into path → content.
func exportVolumeViaAPI(t *testing.T, hostID, volumeName string) map[string][]byte {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/hosts/" + hostID + "/volumes/" + volumeName + "/export")
	require.NoError(t, err, "export for verification must connect")
	defer resp.Body.Close()
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"export %s on %s: expected 200, got %d", volumeName, hostID, resp.StatusCode)
	out := map[string][]byte{}
	tr := tar.NewReader(resp.Body)
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

// Test_Integration_Quadlets_MoveVolumeData pins the volume data migration
// end to end: a migrating copy carries the referenced volume's files to
// the target, a non-empty target volume refuses with 409 (nothing
// installs), overwriteVolumes merges, a dangling reference skips with a
// visible outcome, a move migrates then removes only the source quadlet,
// and the preview reports the migration facts without any flag.
func Test_Integration_Quadlets_MoveVolumeData(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondHost(t)
	if !b.Independent {
		t.Skip("host B shares host A's host (PODLAND_TEST_SSH_* fallback) — volume data migration needs independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("host B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	ensureBusyboxImage(t)

	hostA := suite.HostID
	base := "it-" + suite.RunID + "-mvd"

	// --- AC: a migrating copy carries the volume's data ------------------
	t.Run("MigrateCopyCarriesData", func(t *testing.T) {
		volume := base + "-src"
		quadlet := base + "-copy"
		sourceFiles := map[string][]byte{
			"pg/PG_VERSION": []byte("17"),
			"pg/note.txt":   []byte("carry me"),
		}

		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, sourceFiles)
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + volume + ":/var/lib/pg:Z\n",
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, b.HostID, "copy", nil))

		// The event stream: volume start, volume done, one terminal
		// complete carrying the migrated outcome.
		require.GreaterOrEqual(t, len(events), 3)
		assert.Equal(t, "volume", events[0].Type)
		assert.Equal(t, "start", events[0].Phase)
		assert.Equal(t, volume, events[0].Ref)
		assert.Equal(t, volume, events[0].PodmanVolume)

		done := events[len(events)-2]
		assert.Equal(t, "volume", done.Type)
		assert.Equal(t, "done", done.Phase)

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "copied", complete.Result.Status)
		require.NotNil(t, complete.Result.VolumeMigration)
		require.Len(t, complete.Result.VolumeMigration.Volumes, 1)
		outcome := complete.Result.VolumeMigration.Volumes[0]
		assert.Equal(t, volume, outcome.Ref)
		assert.Equal(t, volume, outcome.PodmanVolume)
		assert.Equal(t, "migrated", outcome.Outcome)

		// The data itself crossed hosts: host B's volume holds the
		// seeded files byte-for-byte.
		onB := exportVolumeViaAPI(t, b.HostID, volume)
		assert.Equal(t, []byte("17"), onB["pg/PG_VERSION"])
		assert.Equal(t, []byte("carry me"), onB["pg/note.txt"])

		// And the quadlet landed on B.
		getQuadlet(t, b.HostID, quadlet+".container")
	})

	// --- AC: a non-empty target volume refuses; nothing installs --------
	t.Run("NonEmptyTargetIs409", func(t *testing.T) {
		volume := base + "-clash"
		quadlet := base + "-clashq"
		sourceFiles := map[string][]byte{"src.txt": []byte("from A")}
		targetFiles := map[string][]byte{"tgt.txt": []byte("already here")}

		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, sourceFiles)
		createVolumeViaAPI(t, b.HostID, volume)
		seedVolumeViaAPI(t, b.HostID, volume, targetFiles)
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + volume + ":/data\n",
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, b.HostID, "copy", nil))

		require.Len(t, events, 1, "the refusal fires before any progress event")
		assert.Equal(t, "error", events[0].Type)
		require.NotNil(t, events[0].Error)
		assert.Equal(t, "VOLUME_MIGRATION_TARGET_NOT_EMPTY", events[0].Error.Code)
		assert.Contains(t, events[0].Error.Message, volume)

		// Nothing installed on either side's quadlet set.
		getQuadletExpectNotFound(t, b.HostID, quadlet+".container")
		quadletListsEntry(t, hostA, quadlet+".container")

		// The target volume keeps its own content untouched.
		onB := exportVolumeViaAPI(t, b.HostID, volume)
		assert.Equal(t, []byte("already here"), onB["tgt.txt"])
		assert.NotContains(t, onB, "src.txt")
	})

	// --- AC: overwriteVolumes merges into the existing target -----------
	t.Run("OverwriteVolumesMerges", func(t *testing.T) {
		volume := base + "-merge"
		quadlet := base + "-mergeq"
		targetFiles := map[string][]byte{"tgt.txt": []byte("already here")}

		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, map[string][]byte{"src.txt": []byte("from A")})
		createVolumeViaAPI(t, b.HostID, volume)
		seedVolumeViaAPI(t, b.HostID, volume, targetFiles)
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + volume + ":/data\n",
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, b.HostID, "copy", map[string]any{"overwriteVolumes": true}))

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "copied", complete.Result.Status)
		require.NotNil(t, complete.Result.VolumeMigration)
		assert.Equal(t, "migrated", complete.Result.VolumeMigration.Volumes[0].Outcome)

		// Merge semantics: both sides' files coexist on B.
		onB := exportVolumeViaAPI(t, b.HostID, volume)
		assert.Equal(t, []byte("already here"), onB["tgt.txt"])
		assert.Equal(t, []byte("from A"), onB["src.txt"])
		getQuadlet(t, b.HostID, quadlet+".container")
	})

	// --- AC: a dangling reference skips with a visible outcome ----------
	t.Run("DanglingReferenceSkips", func(t *testing.T) {
		quadlet := base + "-dangle"
		dangling := base + "-absent"
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + dangling + ".volume:/data\n",
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, b.HostID, "copy", nil))

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "copied", complete.Result.Status, "a dangling reference is not a failure")
		require.NotNil(t, complete.Result.VolumeMigration)
		require.Len(t, complete.Result.VolumeMigration.Volumes, 1)
		outcome := complete.Result.VolumeMigration.Volumes[0]
		assert.Equal(t, dangling+".volume", outcome.Ref)
		assert.Equal(t, "systemd-"+dangling, outcome.PodmanVolume)
		assert.Equal(t, "skipped", outcome.Outcome)
		assert.Equal(t, "source volume does not exist", outcome.Reason)

		// Nothing was created for the dangling resolution on B (the
		// resolved default name exists as a VOLUME neither side), and the
		// quadlet still installed.
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/hosts/"+b.HostID+"/volumes/systemd-"+dangling,
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
		getQuadlet(t, b.HostID, quadlet+".container")
	})

	// --- AC: a migrating move carries data, then removes only the file --
	t.Run("MoveModeMigratesAndRemovesSourceFile", func(t *testing.T) {
		volume := base + "-mv"
		quadlet := base + "-mvq"
		sourceFiles := map[string][]byte{"keep/data.bin": []byte{0x01, 0x02, 0x03}}

		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, sourceFiles)
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + volume + ":/data\n",
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, b.HostID, "move", nil))

		complete := events[len(events)-1]
		require.NotNil(t, complete.Result)
		assert.Equal(t, "moved", complete.Result.Status)
		assert.Equal(t, "migrated", complete.Result.VolumeMigration.Volumes[0].Outcome)

		// The data is on B, the source quadlet is gone, and the source
		// VOLUME stays — a move removes only the quadlet file.
		onB := exportVolumeViaAPI(t, b.HostID, volume)
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, onB["keep/data.bin"])
		getQuadletExpectNotFound(t, hostA, quadlet+".container")
		onA := exportVolumeViaAPI(t, hostA, volume)
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, onA["keep/data.bin"],
			"the volume's data stays behind on the source; only the quadlet moves")
	})

	// --- AC: the preview reports migration facts without any flag -------
	t.Run("PreviewReportsMigrationFacts", func(t *testing.T) {
		volume := base + "-pv"
		quadlet := base + "-pvq"
		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, map[string][]byte{"seed.txt": []byte("facts")})
		for _, host := range []string{hostA, b.HostID} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: "[Container]\nImage=busybox\nVolume=" + volume + ":/data\n" +
				"Volume=/srv/host:/host:ro\n",
		})

		got := getMovePreview(t, hostA, quadlet+".container", b.HostID)
		require.Len(t, got.Volumes, 2)

		vol := got.Volumes[0]
		assert.Equal(t, volume, vol.PodmanVolume)
		assert.True(t, vol.Migratable)
		assert.Empty(t, vol.Reason)
		assert.False(t, vol.ExistsOnTarget, "host B holds nothing of this run")
		assert.Nil(t, vol.SizeBytes, "podman reports no volume usage today")

		host := got.Volumes[1]
		assert.Equal(t, "hostPath", host.Kind)
		assert.False(t, host.Migratable)
		assert.Equal(t, "host bind paths are not migrated", host.Reason)
		assert.Empty(t, host.PodmanVolume)
	})
}
