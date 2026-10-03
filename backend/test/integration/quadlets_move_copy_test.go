//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy transfer slice, copy mode (issue 210). POST
// /api/machines/{id}/quadlets/{quadletId}/move with
// {"targetMachineId": ..., "mode": "copy"} installs the source quadlet's
// file — same name, resource, and content — on the target without
// overwriting and never touches the source (a running source quadlet
// included).
//
// The two-machine guarantee comes from the harness (issue 208): machine B
// is a second testhost container with podman state independent of machine
// A's, so the installed file is physically observable on B. Like every
// quadlet slice this gates on the user-systemd probe (podman's quadlet
// surface dials the user systemd over DBus); the access-denied slice needs
// no podman at all, so it runs wherever the harness runs.

// quadletMoveResponse mirrors the fields of quadlets.QuadletMoveResponse
// that this slice consumes. Kept local so the integration package doesn't
// import the quadlets domain.
type quadletMoveResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Resource        string `json:"resource"`
	Status          string `json:"status"`
	SourceMachineID string `json:"sourceMachineId"`
	TargetMachineID string `json:"targetMachineId"`
	Path            string `json:"path,omitempty"`
	Error           string `json:"error,omitempty"`
}

// moveURL builds the transfer endpoint URL for a source machine's quadlet.
func moveURL(sourceMachineID, quadletName string) string {
	return fmt.Sprintf("%s/api/machines/%s/quadlets/%s/move", suite.ServerURL, sourceMachineID, quadletName)
}

// copyQuadletBody is the JSON body of a copy-mode transfer.
func copyQuadletBody(targetMachineID string) string {
	body, err := json.Marshal(map[string]string{"targetMachineId": targetMachineID, "mode": "copy"})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// copyQuadlet POSTs a copy transfer as the suite user and returns the
// decoded envelope, asserting the 200 shape.
func copyQuadlet(t *testing.T, sourceMachineID, quadletName, targetMachineID string) quadletMoveResponse {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(moveURL(sourceMachineID, quadletName),
		"application/json", bytes.NewReader([]byte(copyQuadletBody(targetMachineID))))
	require.NoError(t, err, "POST /quadlets/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"copy quadlet: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got quadletMoveResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"copy quadlet: response must decode as QuadletMoveResponse. body: %s", string(body))
	return got
}

// copyQuadletExpectConflict POSTs a copy transfer and asserts the 409
// collision answer.
func copyQuadletExpectConflict(t *testing.T, sourceMachineID, quadletName, targetMachineID string) {
	t.Helper()
	expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
		moveURL(sourceMachineID, quadletName), copyQuadletBody(targetMachineID),
		http.StatusConflict, "QUADLET_ALREADY_EXISTS")
}

// quadletListsEntry asserts the machine's quadlet list holds the name and
// returns it, failing the test when absent.
func quadletListsEntry(t *testing.T, machineID, quadletName string) quadletListResponse {
	t.Helper()
	entries := listQuadletsFiltered(t, machineID, url.Values{"name": {quadletName}})
	for _, entry := range entries {
		if entry.Name == quadletName {
			return entry
		}
	}
	t.Fatalf("machine %s must list quadlet %s. entries: %+v", machineID, quadletName, entries)
	return quadletListResponse{}
}

// quadletListNames returns the machine's quadlet names — the shape a
// before/after comparison of a whole list wants (list order is podman's,
// not the contract's, so the names are sorted first).
func quadletListNames(t *testing.T, machineID string, query url.Values) []string {
	t.Helper()
	names := quadletEntryNames(listQuadletsFiltered(t, machineID, query))
	slices.Sort(names)
	return names
}

// Test_Integration_Quadlets_MoveCopy pins the copy contract end to end: an
// identical quadlet lands on machine B (never started), machine A's list is
// unchanged, a second copy collides with 409, a RUNNING source quadlet
// copies without any stop, and all five resource types transfer alike.
func Test_Integration_Quadlets_MoveCopy(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — the copy transfer needs independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("machine B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	ensureBusyboxImage(t)

	machineA := suite.MachineID
	base := "it-" + suite.RunID + "-mvcopy"
	runQuadlet := base + "-run"

	// Per-test cleanup, registered before the creates (404-tolerant like
	// every cleanup here); the suite sweeps are the abort backstop. Both
	// machines are cleaned: a copy lands on B, and every fixture starts on
	// A.
	for _, machine := range []string{machineA, b.MachineID} {
		t.Cleanup(func() { forceDeleteQuadlet(t, machine, base+".container") })
		t.Cleanup(func() { forceDeleteQuadlet(t, machine, runQuadlet+".container") })
		for _, suffix := range []string{"pod", "image", "volume", "network"} {
			t.Cleanup(func() { forceDeleteQuadlet(t, machine, base+"-"+suffix+"."+suffix) })
		}
	}
	t.Cleanup(func() { removeSpawnedContainer(t, runQuadlet) })

	createQuadlet(t, machineA, quadletCreateBody{
		Name:     base,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})

	// --- AC: copy installs an identical quadlet on the target and leaves
	// the source machine's quadlet list unchanged -----------------------
	t.Run("CopyInstallsIdenticalQuadletLeavesSourceUnchanged", func(t *testing.T) {
		// Snapshot machine A's full quadlet list first: "unchanged" is the
		// whole list, not just the fixture still being present.
		before := quadletListNames(t, machineA, nil)

		got := copyQuadlet(t, machineA, base+".container", b.MachineID)

		assert.Equal(t, base+".container", got.ID)
		assert.Equal(t, "container", got.Resource)
		assert.Equal(t, "copied", got.Status)
		assert.Equal(t, machineA, got.SourceMachineID)
		assert.Equal(t, b.MachineID, got.TargetMachineID)

		onB := getQuadlet(t, b.MachineID, base+".container")
		assert.Equal(t, runnableContainerQuadletContent, onB.Content,
			"the target file must carry the source's content verbatim")
		assert.Equal(t, "container", onB.Resource)
		assert.NotEqual(t, "running", onB.State,
			"the transferred unit is installed but never started")

		assert.Equal(t, before, quadletListNames(t, machineA, nil),
			"a copy must not add or remove anything on the source machine")
		onA := quadletListsEntry(t, machineA, base+".container")
		assert.Equal(t, "container", onA.Resource,
			"the source quadlet must still list on machine A with its resource intact")
	})

	// --- AC: a name collision on the target returns 409 and installs
	// nothing ------------------------------------------------------------
	t.Run("CollisionOnTargetReturns409", func(t *testing.T) {
		// Machine B has held the name since the first copy, so this second
		// transfer must be refused — and the refusal must leave B's file
		// intact (still the original content).
		copyQuadletExpectConflict(t, machineA, base+".container", b.MachineID)

		onB := getQuadlet(t, b.MachineID, base+".container")
		assert.Equal(t, runnableContainerQuadletContent, onB.Content,
			"a refused copy must not overwrite the target's existing file")
	})

	// --- AC: copy of a running quadlet succeeds with no stop/force ------
	t.Run("CopyOfRunningQuadletSucceedsWithoutStop", func(t *testing.T) {
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     runQuadlet,
			Resource: "container",
			Content:  runnableContainerQuadletContent,
		})
		startQuadlet(t, machineA, runQuadlet+".container")
		waitForQuadletRunning(t, machineA, runQuadlet+".container",
			"the source quadlet must reach running before the copy")

		got := copyQuadlet(t, machineA, runQuadlet+".container", b.MachineID)
		assert.Equal(t, "copied", got.Status)

		waitForQuadletRunning(t, machineA, runQuadlet+".container",
			"a copied source quadlet must keep running — copy stops nothing")
		quadletListsEntry(t, b.MachineID, runQuadlet+".container")
	})

	// --- AC: all five resource types copy --------------------------------
	// The container type is pinned by the first subtest above (content and
	// resource asserted verbatim on the target); this one walks the
	// remaining four.
	t.Run("AllFiveResourceTypesCopy", func(t *testing.T) {
		// Minimal per-type content: the transfer moves files, it does not
		// generate or validate them, so a bare section header suffices.
		typeOf := map[string]string{
			"pod":     "[Pod]\n",
			"image":   "[Image]\n",
			"volume":  "[Volume]\nDriver=local\n",
			"network": "[Network]\n",
		}
		for _, resource := range []string{"pod", "image", "volume", "network"} {
			name := base + "-" + resource
			createQuadlet(t, machineA, quadletCreateBody{
				Name:     name,
				Resource: resource,
				Content:  typeOf[resource],
			})

			got := copyQuadlet(t, machineA, name+"."+resource, b.MachineID)
			assert.Equal(t, resource, got.Resource)
			assert.Equal(t, "copied", got.Status)

			onB := getQuadlet(t, b.MachineID, name+"."+resource)
			assert.Equal(t, typeOf[resource], onB.Content,
				"the %s quadlet's content must arrive verbatim", resource)
			assert.Equal(t, resource, onB.Resource)
		}
	})
}

// Test_Integration_Quadlets_MoveCopy_TargetAccessDenied pins the AC the
// route middleware cannot express: a caller who may WRITE the source
// machine (so the route's write gate passes) but holds only READ on the
// target gets the standard 403 ACCESS_DENIED from the service's target
// gate — before anything is installed. The gate fires before any podman
// call, so this slice runs on every harness shape, systemd or not.
func Test_Integration_Quadlets_MoveCopy_TargetAccessDenied(t *testing.T) {
	suite.SkipIfUnavailable(t)

	b := suite.RegisterSecondMachine(t)

	// A second, fully authenticated user holding write on machine A (the
	// route gate passes) and read on machine B (below the write the
	// transfer needs) — the same fixture discipline as the preview slice's
	// reader, one level up.
	username := "it-" + suite.RunID + "-mvcw"
	password := "it-" + suite.RunID + "-mvcw-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
	require.NoError(t, err, "seed copy writer user")
	token, err := realLogin(suite.ServerURL, username, password)
	require.NoError(t, err, "login as copy writer user")
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
	// involved), so the 403 below is the target gate, not route-level
	// source access.
	writerGet, err := writer.Get(suite.ServerURL + "/api/machines/" + suite.MachineID + "/")
	require.NoError(t, err)
	writerGet.Body.Close()
	require.Equal(t, http.StatusOK, writerGet.StatusCode,
		"the writer must pass machine A's subtree gate for the 403 below to mean target access")

	// The copy across to machine B is refused with the standard access
	// error — nothing installed, no error shape from the podman layer.
	expectAPIError(t, writer, http.MethodPost,
		moveURL(suite.MachineID, "whatever.container"), copyQuadletBody(b.MachineID),
		http.StatusForbidden, "ACCESS_DENIED")
}
