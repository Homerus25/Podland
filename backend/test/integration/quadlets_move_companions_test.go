//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy companion slice (issue 217). A `Volume=` source
// ending in .volume with no path separator references the same-named
// .volume quadlet FILE on the source host: the preview reports the file
// and its state on the target (absent / identical / differs), and the
// transfer carries the definition along — installed BEFORE the main
// quadlet, skipped when the target already holds it byte-identical,
// refused with 409 when it holds different content. Companions are
// copy-only: a move removes only the main quadlet.
//
// The two-host guarantee comes from the harness (issue 208): host B
// is a second testhost container with podman state independent of host
// A's. Like every quadlet slice this gates on the user-systemd probe
// (podman's quadlet surface dials the user systemd over DBus).

// moveCompanionBody builds a transfer request body with arbitrary extra
// flags (transferVolumeQuadlets above all).
func moveCompanionBody(t *testing.T, targetHostID, mode string, extra map[string]any) string {
	t.Helper()
	payload := map[string]any{"targetHostId": targetHostID, "mode": mode}
	for name, value := range extra {
		payload[name] = value
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(body)
}

// postMove posts a transfer body to the move endpoint and returns the
// response's status and body.
func postMove(t *testing.T, sourceHostID, quadletName, body string) (int, []byte) {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(moveURL(sourceHostID, quadletName),
		"application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err, "POST /quadlets/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// moveCompanionErrorEnvelope mirrors the error shape a refused transfer
// answers with; only the fields this slice asserts are named.
type moveCompanionErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Test_Integration_Quadlets_MoveCompanions pins the companion contract end
// to end: preview states (absent → identical → differs, bare and dangling
// references staying ordinary), the differing-companion 409, a copy
// installing a companion the target lacks, the identical-skip, the
// transferVolumeQuadlets=false opt-out, and a move removing only the main
// quadlet while the companion stays on the source.
func Test_Integration_Quadlets_MoveCompanions(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondHost(t)
	if !b.Independent {
		t.Skip("host B shares host A's host (PODLAND_TEST_SSH_* fallback) — companion transfers need independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("host B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	ensureBusyboxImage(t)

	hostA := suite.HostID
	base := "it-" + suite.RunID + "-mvcomp"
	companion := base + "-data"

	// The main quadlet references the companion by its quadlet filename,
	// plus one bare volume name and one dangling .volume reference — the
	// discovery rule's negative space rides along in the preview.
	companionContent := "[Volume]\nDriver=local\nLabel=org.example.slice=mvcomp\n"
	differingContent := "[Volume]\nDriver=local\nLabel=org.example.slice=other\n"
	mainContent := `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Volume=` + companion + `.volume:/var/lib/data:Z
Volume=` + base + `-plain:/var/lib/plain
Volume=` + base + `-missing.volume:/var/lib/gone
`

	// One main quadlet per scenario, each with its own companion where the
	// scenario needs the target to lack one; base.container shares
	// base-data.volume across the preview and skip scenarios.
	diffQuadlet := base + "-diff"
	copyPair := base + "-fresh"
	noflagPair := base + "-noflag"
	movePair := base + "-mv"

	// Per-test cleanup, registered before the creates (404-tolerant like
	// every cleanup here); the suite sweeps are the abort backstop. Both
	// hosts are cleaned: companions and mains land on B, every fixture
	// starts on A.
	for _, host := range []string{hostA, b.HostID} {
		for _, name := range []string{
			companion + ".volume",
			base + ".container",
			diffQuadlet + ".container",
			copyPair + ".container", copyPair + "-data.volume",
			noflagPair + ".container", noflagPair + "-data.volume",
			movePair + ".container", movePair + "-data.volume",
		} {
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
	}

	createQuadlet(t, hostA, quadletCreateBody{
		Name:     base,
		Resource: "container",
		Content:  mainContent,
	})
	createQuadlet(t, hostA, quadletCreateBody{
		Name:     companion,
		Resource: "volume",
		Content:  companionContent,
	})

	// --- AC: the preview reports quadletFile and targetState per the
	// target's file; bare and dangling references stay ordinary ---------
	t.Run("PreviewReportsCompanionTargetState", func(t *testing.T) {
		// Host B holds nothing of this run: the companion is absent.
		got := getMovePreview(t, hostA, base+".container", b.HostID)
		require.Len(t, got.Volumes, 3)
		assert.Equal(t, companion+".volume", got.Volumes[0].Source)
		assert.Equal(t, companion+".volume", got.Volumes[0].QuadletFile,
			"a suffix reference with a matching source quadlet names the companion file")
		assert.Equal(t, "absent", got.Volumes[0].TargetState)
		assert.Empty(t, got.Volumes[1].QuadletFile,
			"a bare volume name never resolves to a companion file")
		assert.Empty(t, got.Volumes[1].TargetState)
		assert.Empty(t, got.Volumes[2].QuadletFile,
			"a dangling .volume reference carries no companion fields")
		assert.Empty(t, got.Volumes[2].TargetState)

		// Host B gains the byte-identical companion: identical.
		createQuadlet(t, b.HostID, quadletCreateBody{
			Name:     companion,
			Resource: "volume",
			Content:  companionContent,
		})
		got = getMovePreview(t, hostA, base+".container", b.HostID)
		assert.Equal(t, "identical", got.Volumes[0].TargetState)

		// Host B's file changes: differs.
		updateQuadlet(t, b.HostID, companion+".volume", quadletUpdateBody{
			Content: differingContent,
		})
		got = getMovePreview(t, hostA, base+".container", b.HostID)
		assert.Equal(t, "differs", got.Volumes[0].TargetState)
	})

	// --- AC: a differing same-name companion fails 409 before the main
	// quadlet installs; the source stays untouched ----------------------
	t.Run("DifferingCompanionRefuses409BeforeMain", func(t *testing.T) {
		// The fixture references the companion host B currently holds
		// with DIFFERENT content (the previous subtest left it there).
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     diffQuadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + companion + ".volume:/var/lib/data\n",
		})

		status, raw := postMove(t, hostA, diffQuadlet+".container",
			moveCompanionBody(t, b.HostID, "copy", nil))
		require.Equalf(t, http.StatusConflict, status,
			"a differing companion must refuse the transfer. body: %s", string(raw))
		var errResp moveCompanionErrorEnvelope
		require.NoError(t, json.Unmarshal(raw, &errResp))
		assert.Equal(t, "QUADLET_ALREADY_EXISTS", errResp.Error.Code)
		assert.Contains(t, errResp.Error.Message, companion+".volume",
			"the refusal names the companion file")

		getQuadletExpectNotFound(t, b.HostID, diffQuadlet+".container")
		quadletListsEntry(t, hostA, diffQuadlet+".container")
	})

	// --- AC: copy installs a companion the target lacks, before the main
	// quadlet, and leaves the source unchanged --------------------------
	t.Run("CopyInstallsAbsentCompanion", func(t *testing.T) {
		freshContent := "[Container]\nImage=busybox\nVolume=" + copyPair + "-data.volume:/var/lib/data\n"
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     copyPair,
			Resource: "container",
			Content:  freshContent,
		})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     copyPair + "-data",
			Resource: "volume",
			Content:  companionContent,
		})

		status, raw := postMove(t, hostA, copyPair+".container",
			moveCompanionBody(t, b.HostID, "copy", nil))
		require.Equalf(t, http.StatusOK, status, "copy body: %s", string(raw))
		var got quadletMoveResponse
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "copied", got.Status)

		onB := getQuadlet(t, b.HostID, copyPair+"-data.volume")
		assert.Equal(t, companionContent, onB.Content,
			"the companion's definition must arrive verbatim on the target")
		assert.Equal(t, "volume", onB.Resource)
		assert.Equal(t, freshContent, getQuadlet(t, b.HostID, copyPair+".container").Content)

		// The definition travelled, not the file alone: the source keeps
		// both files.
		quadletListsEntry(t, hostA, copyPair+".container")
		quadletListsEntry(t, hostA, copyPair+"-data.volume")
	})

	// --- AC: a companion already on the target byte-identical is skipped
	// and the transfer proceeds -----------------------------------------
	t.Run("IdenticalCompanionSkippedAndProceeds", func(t *testing.T) {
		// Restore host B's companion to the byte-identical content.
		updateQuadlet(t, b.HostID, companion+".volume", quadletUpdateBody{
			Content: companionContent,
		})

		status, raw := postMove(t, hostA, base+".container",
			moveCompanionBody(t, b.HostID, "copy", nil))
		require.Equalf(t, http.StatusOK, status, "copy body: %s", string(raw))
		var got quadletMoveResponse
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "copied", got.Status)

		assert.Equal(t, companionContent, getQuadlet(t, b.HostID, companion+".volume").Content,
			"an identical companion is skipped, not overwritten")
		getQuadlet(t, b.HostID, base+".container")
		quadletListsEntry(t, hostA, base+".container")
	})

	// --- AC: transferVolumeQuadlets=false moves the main quadlet only --
	t.Run("FlagFalseSkipsCompanions", func(t *testing.T) {
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     noflagPair,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + noflagPair + "-data.volume:/var/lib/data\n",
		})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     noflagPair + "-data",
			Resource: "volume",
			Content:  companionContent,
		})

		status, raw := postMove(t, hostA, noflagPair+".container",
			moveCompanionBody(t, b.HostID, "copy", map[string]any{"transferVolumeQuadlets": false}))
		require.Equalf(t, http.StatusOK, status, "copy body: %s", string(raw))
		var got quadletMoveResponse
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "copied", got.Status)

		getQuadlet(t, b.HostID, noflagPair+".container")
		getQuadletExpectNotFound(t, b.HostID, noflagPair+"-data.volume")
	})

	// --- AC: a move does the same and removes only the main quadlet from
	// the source; the companion stays there in every outcome ------------
	t.Run("MoveRemovesOnlyMainQuadlet", func(t *testing.T) {
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     movePair,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nVolume=" + movePair + "-data.volume:/var/lib/data\n",
		})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     movePair + "-data",
			Resource: "volume",
			Content:  companionContent,
		})

		status, raw := postMove(t, hostA, movePair+".container",
			moveCompanionBody(t, b.HostID, "move", nil))
		require.Equalf(t, http.StatusOK, status, "move body: %s", string(raw))
		var got quadletMoveResponse
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "moved", got.Status)

		// The target holds the main quadlet and its companion definition.
		assert.Equal(t, companionContent, getQuadlet(t, b.HostID, movePair+"-data.volume").Content)
		getQuadlet(t, b.HostID, movePair+".container")

		// The source lost the main quadlet only: the companion is a shared
		// definition whose volume data stays behind regardless (#214).
		getQuadletExpectNotFound(t, hostA, movePair+".container")
		quadletListsEntry(t, hostA, movePair+"-data.volume")
	})
}
