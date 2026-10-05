//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cross-machine volume copy/move slice (issue 238): the shared
// transfer kit behind the volumes domain's composition. A copy creates
// the target volume and streams the source's data into it; a move does
// the same and then removes the source VOLUME (unlike a quadlet move,
// which removes only the file). The collision is re-checked at execution
// time (409, never an overwrite), an in-use source refuses a move unless
// force is set, and the preview reports the dialog's facts.
//
// Everything here is API-driven on both machines: volumes are created
// (POST /volumes), seeded (multipart /populate) and verified (untarred
// /export) through the in-process server, so the slice runs wherever the
// two-machine harness runs. No user-systemd probe: the volume surface
// never dials systemd — only the referencing-quadlet preview fact needs a
// quadlet file, and that one subtest gates itself.

// volumeMoveEvent mirrors one NDJSON event of a volume transfer; only the
// fields this slice asserts are named.
type volumeMoveEvent struct {
	OperationID string `json:"operationId"`
	Type        string `json:"type"`
	MachineID   string `json:"machineId"`
	Volume      string `json:"volume,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Result      *struct {
		Name            string `json:"name"`
		Status          string `json:"status"`
		SourceMachineID string `json:"sourceMachineId"`
		TargetMachineID string `json:"targetMachineId"`
		Bytes           int64  `json:"bytes"`
		Error           string `json:"error,omitempty"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// volumeMovePreviewResponse mirrors the preview's JSON shape; only the
// fields this slice asserts are named.
type volumeMovePreviewResponse struct {
	Collision           bool `json:"collision"`
	InUse               bool `json:"inUse"`
	ReferencingQuadlets []struct {
		Name string `json:"name"`
	} `json:"referencingQuadlets"`
}

// parseVolumeMoveEvents decodes the streamed NDJSON body into events.
func parseVolumeMoveEvents(t *testing.T, body string) []volumeMoveEvent {
	t.Helper()
	var events []volumeMoveEvent
	scanner := bufio.NewScanner(bytes.NewReader([]byte(body)))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event volumeMoveEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must decode: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// volumeMoveBody builds a volume transfer request body with arbitrary
// extra flags.
func volumeMoveBody(t *testing.T, targetMachineID, mode string, extra map[string]any) string {
	t.Helper()
	payload := map[string]any{"targetMachineId": targetMachineID, "mode": mode}
	for name, value := range extra {
		payload[name] = value
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(body)
}

// postVolumeMove posts a transfer body to the volume move endpoint and
// returns the response's status and body (the stream answers 200 with any
// failure riding the terminal event).
func postVolumeMove(t *testing.T, machineID, volumeName, body string) (int, []byte) {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+machineID+"/volumes/"+volumeName+"/move",
		"application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err, "POST /volumes/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// postVolumeMoveEvents posts a transfer body and returns the parsed event
// stream, requiring the stream itself to answer 200.
func postVolumeMoveEvents(t *testing.T, machineID, volumeName, body string) []volumeMoveEvent {
	t.Helper()
	status, raw := postVolumeMove(t, machineID, volumeName, body)
	require.Equalf(t, http.StatusOK, status,
		"the volume transfer's stream itself must succeed. body: %s", string(raw))
	return parseVolumeMoveEvents(t, string(raw))
}

// postVolumeMoveEventsVia posts a transfer body through the given
// authenticated client and base URL — the bounded second instance's
// variant of postVolumeMoveEvents — and returns the parsed event stream.
func postVolumeMoveEventsVia(t *testing.T, client *http.Client, baseURL, machineID, volumeName, body string) []volumeMoveEvent {
	t.Helper()
	resp, err := client.Post(
		baseURL+"/api/machines/"+machineID+"/volumes/"+volumeName+"/move",
		"application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err, "POST /volumes/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"the volume transfer's stream itself must succeed. body: %s", string(raw))
	return parseVolumeMoveEvents(t, string(raw))
}

// getVolumeMovePreview GETs the move preview and returns the decoded
// response.
func getVolumeMovePreview(t *testing.T, machineID, volumeName, targetMachineID string) volumeMovePreviewResponse {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/machines/" + machineID + "/volumes/" + volumeName +
			"/move-preview?targetMachineId=" + targetMachineID)
	require.NoError(t, err, "GET move-preview must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"move preview: expected 200, got %d. body: %s", resp.StatusCode, string(raw))
	var got volumeMovePreviewResponse
	require.NoErrorf(t, json.Unmarshal(raw, &got),
		"move preview must decode. body: %s", string(raw))
	return got
}

// cleanupVolume registers a best-effort API deletion of a volume — the
// fixture discipline for volumes this slice creates outside
// createVolumeViaAPI (the transfer's own target-side creations).
func cleanupVolume(t *testing.T, machineID, volumeName string) {
	t.Helper()
	t.Cleanup(func() {
		resp, err := suite.AuthenticatedClient().Do(standardRequest(t, http.MethodDelete,
			suite.ServerURL+"/api/machines/"+machineID+"/volumes/"+volumeName+"?force=true"))
		if err != nil {
			t.Logf("cleanup volume %s on %s: %v", volumeName, machineID, err)
			return
		}
		resp.Body.Close()
	})
}

// Test_Integration_Volumes_Move pins the cross-machine volume copy/move
// contract end to end: a copy carries the data and leaves the source, a
// move carries the data and removes the source VOLUME, a same-named
// target volume refuses with 409 (target content untouched), an in-use
// source refuses a move unless forced, and the preview reports the
// dialog's facts.
func Test_Integration_Volumes_Move(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()
	registerContainersSweep()
	registerQuadletsSweep()

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — volume transfers need independent podman state")
	}

	machineA := suite.MachineID
	base := "it-" + suite.RunID + "-vmv"

	// --- AC: a copy carries the data, the source stays ------------------
	t.Run("CopyCarriesData", func(t *testing.T) {
		volume := base + "-copy"
		sourceFiles := map[string][]byte{
			"pg/PG_VERSION": []byte("17"),
			"pg/note.txt":   []byte("carry me"),
		}

		createVolumeViaAPI(t, machineA, volume)
		seedVolumeViaAPI(t, machineA, volume, sourceFiles)
		cleanupVolume(t, b.MachineID, volume)

		events := postVolumeMoveEvents(t, machineA, volume,
			volumeMoveBody(t, b.MachineID, "copy", nil))

		// The event stream: volume start, volume done, one terminal
		// complete carrying the copied envelope.
		require.GreaterOrEqual(t, len(events), 3)
		assert.Equal(t, "volume", events[0].Type)
		assert.Equal(t, "start", events[0].Phase)
		assert.Equal(t, volume, events[0].Volume)
		assert.Equal(t, machineA, events[0].MachineID)
		assert.NotEmpty(t, events[0].OperationID, "the transport stamps its operation ID")

		done := events[len(events)-2]
		assert.Equal(t, "volume", done.Type)
		assert.Equal(t, "done", done.Phase)

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "copied", complete.Result.Status)
		assert.Equal(t, volume, complete.Result.Name)
		assert.Equal(t, machineA, complete.Result.SourceMachineID)
		assert.Equal(t, b.MachineID, complete.Result.TargetMachineID)
		assert.Greater(t, complete.Result.Bytes, int64(0))

		// The data itself crossed machines byte-for-byte, and the
		// source volume keeps its copy — a copy never touches the
		// source.
		onB := exportVolumeViaAPI(t, b.MachineID, volume)
		assert.Equal(t, []byte("17"), onB["pg/PG_VERSION"])
		assert.Equal(t, []byte("carry me"), onB["pg/note.txt"])
		onA := exportVolumeViaAPI(t, machineA, volume)
		assert.Equal(t, []byte("carry me"), onA["pg/note.txt"])
	})

	// --- AC: a move carries the data, then removes the source VOLUME ----
	t.Run("MoveRemovesSourceVolume", func(t *testing.T) {
		volume := base + "-mv"
		sourceFiles := map[string][]byte{"keep/data.bin": []byte{0x01, 0x02, 0x03}}

		createVolumeViaAPI(t, machineA, volume)
		seedVolumeViaAPI(t, machineA, volume, sourceFiles)
		cleanupVolume(t, b.MachineID, volume)

		events := postVolumeMoveEvents(t, machineA, volume,
			volumeMoveBody(t, b.MachineID, "move", nil))

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "moved", complete.Result.Status)
		assert.Empty(t, complete.Result.Error, "a clean move reports no partial detail")

		// The data is on B and the source VOLUME is gone — a volume
		// move removes the volume itself, unlike a quadlet move.
		onB := exportVolumeViaAPI(t, b.MachineID, volume)
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, onB["keep/data.bin"])
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/machines/"+machineA+"/volumes/"+volume,
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
	})

	// --- AC: a same-named target volume refuses; nothing is overwritten -
	t.Run("CollisionIs409", func(t *testing.T) {
		volume := base + "-clash"
		sourceFiles := map[string][]byte{"src.txt": []byte("from A")}
		targetFiles := map[string][]byte{"tgt.txt": []byte("already here")}

		createVolumeViaAPI(t, machineA, volume)
		seedVolumeViaAPI(t, machineA, volume, sourceFiles)
		createVolumeViaAPI(t, b.MachineID, volume)
		seedVolumeViaAPI(t, b.MachineID, volume, targetFiles)

		events := postVolumeMoveEvents(t, machineA, volume,
			volumeMoveBody(t, b.MachineID, "copy", nil))

		require.Len(t, events, 1, "the refusal fires before any progress event")
		assert.Equal(t, "error", events[0].Type)
		require.NotNil(t, events[0].Error)
		assert.Equal(t, "VOLUME_ALREADY_EXISTS", events[0].Error.Code)
		assert.Contains(t, events[0].Error.Message, volume)

		// The target volume keeps its own content untouched.
		onB := exportVolumeViaAPI(t, b.MachineID, volume)
		assert.Equal(t, []byte("already here"), onB["tgt.txt"])
		assert.NotContains(t, onB, "src.txt")
	})

	// --- AC: an in-use source refuses a move; force proceeds -------------
	t.Run("InUseMoveRefusedThenForced", func(t *testing.T) {
		volume := base + "-inuse"
		containerName := base + "-inuse-c"

		createVolumeViaAPI(t, machineA, volume)
		seedVolumeViaAPI(t, machineA, volume, map[string][]byte{"data.txt": []byte("live")})
		cleanupVolume(t, b.MachineID, volume)
		t.Cleanup(func() {
			_, _ = podmanConnRun("rm", "-f", containerName)
		})

		// Fixture: a running container mounting the volume — the
		// delete slice's in-use fixture, the derivation's proof against
		// real podman.
		ensureBusyboxImage(t)
		out, err := podmanConnRun("run", "-d", "--name", containerName,
			"-v", volume+":/data", "busybox:latest", "sh", "-c", "sleep 600")
		require.NoError(t, err, "fixture container create must succeed: %s", out)

		// Without force: the move refuses with the in-use code, before
		// anything is created on the target.
		events := postVolumeMoveEvents(t, machineA, volume,
			volumeMoveBody(t, b.MachineID, "move", nil))
		require.Len(t, events, 1, "the refusal fires before any progress event")
		assert.Equal(t, "error", events[0].Type)
		require.NotNil(t, events[0].Error)
		assert.Equal(t, "VOLUME_IN_USE", events[0].Error.Code)
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/machines/"+b.MachineID+"/volumes/"+volume,
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")

		// With force: the transfer proceeds; podman stops the dependent
		// container and removes the source volume.
		events = postVolumeMoveEvents(t, machineA, volume,
			volumeMoveBody(t, b.MachineID, "move", map[string]any{"force": true}))
		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "moved", complete.Result.Status)

		onB := exportVolumeViaAPI(t, b.MachineID, volume)
		assert.Equal(t, []byte("live"), onB["data.txt"])
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/machines/"+machineA+"/volumes/"+volume,
			"", http.StatusNotFound, "VOLUME_NOT_FOUND")
		assert.NotEqual(t, "running", containerStateByName(t, containerName),
			"podman must have stopped the dependent container")
	})

	// --- AC: the preview reports the dialog's facts ----------------------
	t.Run("PreviewReportsFacts", func(t *testing.T) {
		volume := base + "-pv"
		quadlet := base + "-pvq"

		// The referencing-quadlet fact needs a quadlet file on the
		// source machine — the quadlet surface's systemd probe, gated
		// to this subtest alone.
		requireUserSystemd(t)

		createVolumeViaAPI(t, machineA, volume)
		seedVolumeViaAPI(t, machineA, volume, map[string][]byte{"seed.txt": []byte("facts")})
		t.Cleanup(func() { forceDeleteQuadlet(t, machineA, quadlet+".container") })
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + volume + ":/data\n",
		})

		got := getVolumeMovePreview(t, machineA, volume, b.MachineID)
		assert.False(t, got.Collision, "machine B holds nothing of this run")
		assert.False(t, got.InUse, "no container mounts the volume")
		require.Len(t, got.ReferencingQuadlets, 1)
		assert.Equal(t, quadlet+".container", got.ReferencingQuadlets[0].Name)

		// A same-named volume appearing on the target flips the
		// collision flag — the fact the dialog's confirm gate reads.
		createVolumeViaAPI(t, b.MachineID, volume)
		got = getVolumeMovePreview(t, machineA, volume, b.MachineID)
		assert.True(t, got.Collision, "a same-named target volume is a collision")
	})
}

// bootVolumeBoundedApp boots a second in-process backend whose volume
// transfer bound is tiny, so the rollback path can be exercised for real
// against both test hosts without touching the shared suite server's
// production-shaped 2 GiB cap — the ratelimit slices' second-instance
// pattern (ratelimit_auth_test.go). Tear-down mirrors the suite's LIFO
// order: server close → app close → temp files (t.TempDir's cleanup fires
// last).
func bootVolumeBoundedApp(t *testing.T, maxUploadBytes int64) (*app.App, string) {
	t.Helper()

	cfg, err := buildTestConfig(t.TempDir())
	require.NoError(t, err, "build the bounded instance's config")
	cfg.Volumes.MaxUploadBytes = maxUploadBytes

	application, err := app.New(cfg)
	require.NoError(t, err, "boot the app with the %d-byte transfer bound", maxUploadBytes)

	srv := httptest.NewServer(application.Handler())
	t.Cleanup(func() { _ = application.Close() })
	t.Cleanup(srv.Close)
	return application, srv.URL
}

// Test_Integration_Volumes_Move_BoundTripsAndRollsBack pins the rollback
// end to end (issue 238's AC): a transfer whose data crosses the wired
// bound fails AFTER the target volume's creation, and the half-populated
// volume is removed again — a partial volume must not survive to block a
// retry with a false collision. The bound trip is the one rollback
// trigger reachable through the real podman surface (an import failure
// cannot be simulated without breaking the machine, so that trigger
// stays pinned at the service seam); crossing the shared suite server's
// 2 GiB bound would need 2 GiB of data, so the transfer runs against a
// second app instance whose bound is tiny, with both test hosts
// registered in its own DB through the public API. The volume itself is
// created, seeded and verified through the shared suite server: the
// physical volume lives on the test host either way.
func Test_Integration_Volumes_Move_BoundTripsAndRollsBack(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — volume transfers need independent podman state")
	}

	// The bounded instance: a 1 KiB transfer bound, so a 64 KiB volume
	// crosses it mid-stream — after the target volume's creation.
	const bound = 1 << 10
	application, serverURL := bootVolumeBoundedApp(t, bound)

	// Its own caller and both machines, registered through its public API
	// like any production machine (encryption-at-rest, owner-access
	// grant, connection probe).
	username := "it-" + suite.RunID + "-vmvb"
	password := "it-" + suite.RunID + "-vmvb-pw-" + uuid.New().String()[:8]
	_, err := application.SeedUser(username, password)
	require.NoError(t, err, "seed the bounded instance's user")
	token, err := realLogin(serverURL, username, password)
	require.NoError(t, err, "login to the bounded instance")
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
	}
	boundedA, err := registerMachine(serverURL, token, machineRegistration{
		Name:    "it-" + suite.RunID + "-vmvb-a",
		Host:    suite.goodMachineRegistration.Host,
		Port:    suite.goodMachineRegistration.Port,
		SSHUser: suite.goodMachineRegistration.SSHUser,
		Socket:  suite.goodMachineRegistration.Socket,
		SSHKey:  suite.goodMachineRegistration.SSHKey,
	})
	require.NoError(t, err, "register machine A in the bounded instance")
	boundedB, err := registerMachine(serverURL, token, machineRegistration{
		Name:    "it-" + suite.RunID + "-vmvb-b",
		Host:    b.Target.Host,
		Port:    b.Target.Port,
		SSHUser: b.Target.SSHUser,
		Socket:  b.Target.Socket,
		SSHKey:  b.Target.SSHKey,
	})
	require.NoError(t, err, "register machine B in the bounded instance")

	// The source volume: created and seeded through the shared suite
	// server (whose populate cap is the 2 GiB default), far larger than
	// the bounded instance's transfer bound.
	volume := "it-" + suite.RunID + "-vmv-rb"
	payload := bytes.Repeat([]byte("x"), 64<<10)
	createVolumeViaAPI(t, suite.MachineID, volume)
	seedVolumeViaAPI(t, suite.MachineID, volume, map[string][]byte{"bulk/payload.bin": payload})
	cleanupVolume(t, b.MachineID, volume)

	// The transfer through the bounded instance: the bound trips
	// mid-stream, after the target volume's creation.
	events := postVolumeMoveEventsVia(t, client, serverURL, boundedA, volume,
		volumeMoveBody(t, boundedB, "copy", nil))

	// The start event proves the target volume was created — the
	// rollback had something to remove.
	require.GreaterOrEqual(t, len(events), 2,
		"the bound trips after the target creation, so a start event precedes the terminal error")
	assert.Equal(t, "volume", events[0].Type)
	assert.Equal(t, "start", events[0].Phase)

	// The terminal error event carries the 413's vocabulary, and its
	// message states the rollback.
	terminal := events[len(events)-1]
	assert.Equal(t, "error", terminal.Type)
	require.NotNil(t, terminal.Error)
	assert.Equal(t, "VOLUME_UPLOAD_TOO_LARGE", terminal.Error.Code)
	assert.Contains(t, terminal.Error.Message, "partially transferred target volume was removed")

	// The rollback itself, against real podman: the target volume is gone
	// on machine B — no partial volume survives to block a retry with a
	// false collision.
	expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
		suite.ServerURL+"/api/machines/"+b.MachineID+"/volumes/"+volume,
		"", http.StatusNotFound, "VOLUME_NOT_FOUND")

	// The source volume keeps its data — a copy never touches the source.
	onA := exportVolumeViaAPI(t, suite.MachineID, volume)
	assert.Equal(t, payload, onA["bulk/payload.bin"])
}

// Test_Integration_Volumes_Move_TargetAccessDenied pins the AC the route
// middleware cannot express: a caller who may WRITE the source machine
// (so the route's write gate passes) but holds only READ on the target.
// The move's target write gate refuses through the stream's terminal
// error event (the gate fires inside the operation, after the NDJSON
// envelope began); the preview's target read gate lets the same caller
// through. Both gates fire before any podman call on the target, so
// this slice runs on every harness shape, systemd or not.
func Test_Integration_Volumes_Move_TargetAccessDenied(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerVolumesSweep()

	b := suite.RegisterSecondMachine(t)

	// A second, fully authenticated user holding write on machine A (the
	// route gate passes) and read on machine B (below the write the
	// transfer needs) — the quadlet access slice's fixture discipline.
	username := "it-" + suite.RunID + "-vmvw"
	password := "it-" + suite.RunID + "-vmvw-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
	require.NoError(t, err, "seed volume move writer user")
	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "login as volume move writer user")
	writer := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
	}

	// The suite user owns both machines; grant the writer write on A and
	// read on B.
	for _, grant := range []struct {
		machineID  string
		permission string
	}{
		{machineID: suite.MachineID, permission: models.PermissionWrite},
		{machineID: b.MachineID, permission: models.PermissionRead},
	} {
		grantBody, err := json.Marshal(map[string]string{
			"userId":     userID,
			"permission": grant.permission,
		})
		require.NoError(t, err)
		resp, err := suite.AuthenticatedClient().Post(
			suite.ServerURL+"/api/machines/"+grant.machineID+"/access",
			"application/json", bytes.NewReader(grantBody))
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equalf(t, http.StatusCreated, resp.StatusCode,
			"grant %s on %s: expected 201, got %d. body: %s",
			grant.permission, grant.machineID, resp.StatusCode, string(raw))
		t.Cleanup(func() {
			req, err := http.NewRequest(http.MethodDelete,
				suite.ServerURL+"/api/machines/"+grant.machineID+"/access/"+userID, nil)
			if err != nil {
				return
			}
			if resp, err := suite.AuthenticatedClient().Do(req); err == nil {
				resp.Body.Close()
			}
		})
	}

	// Positive control: the same caller reads machine A itself (no podman
	// involved), so the refusal below is the target gate, not route-level
	// source access.
	writerGet, err := writer.Get(suite.ServerURL + "/api/machines/" + suite.MachineID + "/")
	require.NoError(t, err)
	writerGet.Body.Close()
	require.Equal(t, http.StatusOK, writerGet.StatusCode,
		"the writer must pass machine A's subtree gate for the refusal below to mean target access")

	// The move across to machine B is refused with the standard access
	// error riding the stream's terminal event — nothing transferred, no
	// error shape from the podman layer.
	moveResp, err := writer.Post(
		suite.ServerURL+"/api/machines/"+suite.MachineID+"/volumes/whatever/move",
		"application/json", bytes.NewReader([]byte(volumeMoveBody(t, b.MachineID, "copy", nil))))
	require.NoError(t, err)
	defer moveResp.Body.Close()
	raw, _ := io.ReadAll(moveResp.Body)
	require.Equalf(t, http.StatusOK, moveResp.StatusCode,
		"the stream itself must succeed; the refusal rides the terminal event. body: %s", string(raw))
	events := parseVolumeMoveEvents(t, string(raw))
	require.Len(t, events, 1, "the target gate refuses before any progress event")
	assert.Equal(t, "error", events[0].Type)
	require.NotNil(t, events[0].Error)
	assert.Equal(t, "ACCESS_DENIED", events[0].Error.Code)

	// The same caller's preview passes: read on the target suffices
	// there, and the source volume exists (the suite user's fixture).
	volume := "it-" + suite.RunID + "-vmv-acs"
	createVolumeViaAPI(t, suite.MachineID, volume)
	previewResp, err := writer.Get(
		suite.ServerURL + "/api/machines/" + suite.MachineID + "/volumes/" + volume +
			"/move-preview?targetMachineId=" + b.MachineID)
	require.NoError(t, err)
	defer previewResp.Body.Close()
	raw, _ = io.ReadAll(previewResp.Body)
	require.Equalf(t, http.StatusOK, previewResp.StatusCode,
		"read on the target must satisfy the preview's gate. body: %s", string(raw))
}
