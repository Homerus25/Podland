//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy preview slice (issue 209). GET
// /api/machines/{id}/quadlets/{quadletId}/move-preview?targetMachineId=…
// answers everything the transfer dialog must show before confirming:
// the parsed Volume= references with their kind labels, the target
// collision flag, and the source quadlet's running state.
//
// The two-machine guarantee comes from the harness (issue 208): machine B
// is a second testhost container with podman state independent of machine
// A's. Like every quadlet slice this gates on the user-systemd probe
// (podman's quadlet surface dials the user systemd over DBus); the
// access-denied slice needs no podman at all, so it runs wherever the
// harness runs.

// quadletMovePreviewVolume mirrors one entry of the preview's volumes.
type quadletMovePreviewVolume struct {
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	QuadletFile string `json:"quadletFile,omitempty"`
	TargetState string `json:"targetState,omitempty"`
	// Issue 214's flag-independent migration facts.
	PodmanVolume    string `json:"podmanVolume,omitempty"`
	SizeBytes       *int64 `json:"sizeBytes,omitempty"`
	ExistsOnTarget  bool   `json:"existsOnTarget"`
	TargetSizeBytes *int64 `json:"targetSizeBytes,omitempty"`
	Migratable      bool   `json:"migratable"`
	Reason          string `json:"reason,omitempty"`
}

// quadletMovePreviewSecret mirrors one entry of the preview's secrets
// array (issue 215).
type quadletMovePreviewSecret struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	ExistsOnSource bool   `json:"existsOnSource"`
	ExistsOnTarget bool   `json:"existsOnTarget"`
}

// quadletMovePreviewResponse mirrors the fields of
// quadlets.QuadletMovePreviewResponse that this slice consumes. Kept
// local so the integration package doesn't import the quadlets domain.
type quadletMovePreviewResponse struct {
	Volumes   []quadletMovePreviewVolume `json:"volumes"`
	Secrets   []quadletMovePreviewSecret `json:"secrets"`
	Collision bool                       `json:"collision"`
	Running   bool                       `json:"running"`
}

// getMovePreview GETs the move-preview endpoint and returns the decoded
// response, asserting the 200 shape.
func getMovePreview(t *testing.T, sourceMachineID, quadletName, targetMachineID string) quadletMovePreviewResponse {
	t.Helper()
	url := fmt.Sprintf("%s/api/machines/%s/quadlets/%s/move-preview?targetMachineId=%s",
		suite.ServerURL, sourceMachineID, quadletName, targetMachineID)
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /quadlets/{id}/move-preview must connect to the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"move preview: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got quadletMovePreviewResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"move preview: response must decode as QuadletMovePreviewResponse. body: %s", string(body))
	return got
}

// Test_Integration_Quadlets_MovePreview pins the preview contract end to
// end: volumes parsed from the quadlet content with volume/hostPath
// kinds, the collision flag flipping exactly when the target holds a
// quadlet of the same name and resource, and running mirroring the
// source quadlet's live state.
//
// Two quadlets do the work: one with Volume= lines (parsed, collided,
// never started) and one without (started, proving the running flag and
// the empty-volume list). Keeping the started quadlet free of Volume=
// lines avoids binding host paths that don't exist on the test host.
func Test_Integration_Quadlets_MovePreview(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — collision semantics need independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("machine B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	ensureBusyboxImage(t)

	machineA := suite.MachineID
	volumeQuadlet := "it-" + suite.RunID + "-mvprev"
	plainQuadlet := "it-" + suite.RunID + "-mvprev-run"

	volumeContent := `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Volume=` + volumeQuadlet + `-data:/var/lib/data:Z
Volume=/srv/` + volumeQuadlet + `-backup:/backup:ro
`

	// Per-test cleanup, registered before the creates (404-tolerant like
	// every cleanup here); the suite sweep is the abort backstop.
	t.Cleanup(func() { forceDeleteQuadlet(t, machineA, volumeQuadlet+".container") })
	t.Cleanup(func() { forceDeleteQuadlet(t, machineA, plainQuadlet+".container") })
	t.Cleanup(func() { removeSpawnedContainer(t, plainQuadlet) })
	t.Cleanup(func() { forceDeleteQuadlet(t, b.MachineID, volumeQuadlet+".container") })

	createQuadlet(t, machineA, quadletCreateBody{
		Name:     volumeQuadlet,
		Resource: "container",
		Content:  volumeContent,
	})

	// --- AC: every Volume= line returned with the right kind, collision
	// and running false on a clean target -----------------------------
	t.Run("PreviewListsVolumesWithKinds", func(t *testing.T) {
		got := getMovePreview(t, machineA, volumeQuadlet+".container", b.MachineID)

		require.Len(t, got.Volumes, 2)
		assert.Equal(t, volumeQuadlet+"-data", got.Volumes[0].Source)
		assert.Equal(t, "volume", got.Volumes[0].Kind,
			"a source without a path separator is a named podman volume")
		assert.Equal(t, "/srv/"+volumeQuadlet+"-backup", got.Volumes[1].Source)
		assert.Equal(t, "hostPath", got.Volumes[1].Kind,
			"a source with a path separator is a host bind path")
		assert.False(t, got.Collision, "machine B's quadlet list is empty for this run")
		assert.False(t, got.Running, "the never-started source quadlet is not running")
	})

	// --- AC: collision true exactly when the target has a quadlet of the
	// same name and resource ------------------------------------------
	t.Run("CollisionFlagsWhenTargetHasSameNameAndResource", func(t *testing.T) {
		// Absence was proven in the previous subtest, so this flip is real.
		createQuadlet(t, b.MachineID, quadletCreateBody{
			Name:     volumeQuadlet,
			Resource: "container",
			Content:  volumeContent,
		})

		got := getMovePreview(t, machineA, volumeQuadlet+".container", b.MachineID)
		assert.True(t, got.Collision, "the target now holds a quadlet of the same name and resource")
	})

	// --- AC: running reflects the source quadlet's live state; a quadlet
	// without Volume= lines reports an empty volume list ---------------
	t.Run("RunningReflectsSourceLiveState", func(t *testing.T) {
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     plainQuadlet,
			Resource: "container",
			Content:  runnableContainerQuadletContent,
		})
		startQuadlet(t, machineA, plainQuadlet+".container")
		waitForQuadletRunning(t, machineA, plainQuadlet+".container",
			"the source quadlet must reach running before the preview's running flag is polled")

		got := getMovePreview(t, machineA, plainQuadlet+".container", b.MachineID)
		assert.True(t, got.Running, "the started source quadlet's live state is running")
		assert.NotNil(t, got.Volumes, "a quadlet without Volume= lines must report an empty list, not null")
		assert.Empty(t, got.Volumes)
	})
}

// Test_Integration_Quadlets_MovePreview_TargetAccessDenied pins the AC the
// route middleware cannot express: a caller who may read the source machine
// but holds no access row on the target gets the standard 403
// ACCESS_DENIED from the service's target gate — never a false preview.
// The gate fires before any podman call, so this slice runs on every
// harness shape, systemd or not.
func Test_Integration_Quadlets_MovePreview_TargetAccessDenied(t *testing.T) {
	suite.SkipIfUnavailable(t)

	b := suite.RegisterSecondMachine(t)

	// A second, fully authenticated user holding read on machine A and
	// nothing on machine B — the same fixture discipline as the
	// machine-access slice's no-access user, minus the login shortcut.
	username := "it-" + suite.RunID + "-mvread"
	password := "it-" + suite.RunID + "-mvread-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
	require.NoError(t, err, "seed preview reader user")
	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "login as preview reader user")
	reader := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
	}

	// The suite user owns machine A; grant the reader read access there.
	grantBody, err := json.Marshal(map[string]string{
		"userId":     userID,
		"permission": models.PermissionRead,
	})
	require.NoError(t, err)
	grantResp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+suite.MachineID+"/access",
		"application/json", bytes.NewReader(grantBody))
	require.NoError(t, err)
	grantRaw, _ := io.ReadAll(grantResp.Body)
	grantResp.Body.Close()
	require.Equalf(t, http.StatusCreated, grantResp.StatusCode,
		"grant read on machine A: expected 201, got %d. body: %s", grantResp.StatusCode, string(grantRaw))
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/access/"+userID, nil)
		if err != nil {
			return
		}
		if resp, err := suite.AuthenticatedClient().Do(req); err == nil {
			resp.Body.Close()
		}
	})

	// Positive control: the same caller reads machine A itself (no podman
	// involved), so the 403 below is the target gate, not route-level
	// source access.
	readerGet, err := reader.Get(suite.ServerURL + "/api/machines/" + suite.MachineID + "/")
	require.NoError(t, err)
	readerGet.Body.Close()
	require.Equal(t, http.StatusOK, readerGet.StatusCode,
		"the reader must pass machine A's subtree gate for the 403 below to mean target access")

	// The preview across to machine B is refused with the standard
	// access error — no preview body, no false collision=false.
	expectAPIError(t, reader, http.MethodGet,
		suite.ServerURL+"/api/machines/"+suite.MachineID+"/quadlets/whatever.container/move-preview?targetMachineId="+b.MachineID,
		"", http.StatusForbidden, "ACCESS_DENIED")
}
