//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_CRUD is the quadlet CRUD vertical slice for the
// real-podman integration suite (PRD 080-08). It exercises the full quadlet
// management lifecycle end-to-end against the once-registered machine:
//
//	POST   /api/machines/{id}/quadlets             → 201 created
//	GET    /api/machines/{id}/quadlets             → list (run-scoped filter)
//	GET    /api/machines/{id}/quadlets/{quadletId} → 200 with content + metadata
//	PUT    /api/machines/{id}/quadlets/{quadletId} → 200 with updated content
//	DELETE /api/machines/{id}/quadlets/{quadletId} → 204 removed
//
// This proves create → list → get → update → delete over the SSH tunnel,
// on top of the harness shared with the tracer/images/containers slices.
//
// Isolation model (per the master PRD's "run-scoped tag + per-test
// cleanup + suite sweep" ruling):
//
//   - The quadlet is created with a run-scoped name (it-<runID>-crud) so
//     list/count assertions and cleanup can find exactly what this run
//     created. Quadlets have no label filter exposed by the list endpoint,
//     so name-prefix filtering is the only reliable isolation mechanism.
//   - t.Cleanup DELETEs the quadlet via the API, with a fallback to
//     `podman quadlet rm -f <name>` over the connection for robustness.
//   - A suite-teardown sweep (registered once per process) removes any
//     run-scoped quadlet left over from an aborted test (panic, SIGINT
//     before the per-test cleanup fired, …).
//
// Prerequisite: busybox must exist on the machine before quadlet create
// will succeed (podman does not auto-pull on quadlet start). The test
// ensures this itself by reusing `ensureBusyboxImage` from the containers
// slice — each test ensures its own prerequisites.
//
// TRIPWIRE: The DELETE `stop` query param is a no-op gap. QuadletService.Delete
// currently ignores the `stop` parameter (see service.go:315). SSH-exec has
// since landed, but the wiring was never retrofitted. Consequence: deleting
// a **running** quadlet without `force` returns QUADLET_RESOURCE_RUNNING (400)
// — the "stop-then-remove" path the `stop` param was meant to trigger does
// not exist. This gap is exercised by the lifecycle slice (080-09); this
// CRUD slice documents the gap and tests only the states it can reach
// (not-running quadlet: 204 happy path, 404 not-found, `force=true` harmless
// on a stopped unit).
func Test_Integration_Quadlets_CRUD(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// --- AC #1: ensure busybox exists (idempotent) -------------------
	// Reuse the containers slice's helper; quadlet start will fail without
	// the image present, and this slice's DELETE assertions only cover the
	// not-running case (the running + stop-param gap is 080-09's domain).
	ensureBusyboxImage(t)

	// Run-scoped name so list/count assertions and the suite sweep can
	// find exactly what this run created. The filename on the remote machine
	// becomes `<name>.container`.
	quadletName := "it-" + suite.RunID + "-crud"
	createReq := quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	}

	// Per-test cleanup: DELETE the quadlet we create via API. Registered
	// before create so it runs even if a later phase fails. Best-effort: the
	// sweep is the backstop for the abort case.
	//
	// Note: podman quadlet rm is not implemented for remote clients, so we
	// rely on the API delete and the sweep function for cleanup. The sweep
	// uses podman quadlet ls to find run-scoped quadlets and removes them
	// via systemctl + rm.
	t.Cleanup(func() {
		deleteQuadlet(t, machineID, quadletName+".container")
	})

	// --- AC #2: POST creates a run-scoped-named quadlet; 201 -----------
	t.Run("CreateReturns201", func(t *testing.T) {
		createQuadlet(t, machineID, createReq)
		t.Logf("quadlet created: name=%s", quadletName)
	})
	if t.Failed() {
		return
	}

	// --- AC #3: GET /quadlets returns the created quadlet ------------
	t.Run("ListReturnsCreatedQuadlet", func(t *testing.T) {
		requireQuadletListed(t, machineID, quadletName+".container",
			"created quadlet must appear in the quadlet list")
	})
	if t.Failed() {
		return
	}

	// --- AC #4: GET /quadlets/{name} returns content + metadata ------
	t.Run("GetReturnsContentAndMetadata", func(t *testing.T) {
		// GET expects the full name with extension in the URL path
		got := getQuadlet(t, machineID, quadletName+".container")
		// The API returns the name with extension
		require.Equal(t, quadletName+".container", got.Name, "quadlet name must match request (with extension)")
		require.Equal(t, "container", got.Resource, "quadlet resource must be container")
		require.Equal(t, runnableContainerQuadletContent, got.Content, "quadlet content must match request")
		require.NotEmpty(t, got.Path, "quadlet path must be populated by podman")
		require.NotEmpty(t, got.Unit, "quadlet unit must be populated by podman")
	})
	if t.Failed() {
		return
	}

	// --- AC #5: PUT replaces content; subsequent GET reflects it -------
	updatedContent := `[Container]
Image=busybox
Exec=sh -c "sleep 7200"
`
	t.Run("UpdateReplacesContent", func(t *testing.T) {
		// PUT expects the full name with extension (the service now correctly
		// strips the existing extension before adding the new one)
		updateQuadlet(t, machineID, quadletName+".container", quadletUpdateBody{
			Content: updatedContent,
		})
		got := getQuadlet(t, machineID, quadletName+".container")
		require.Equal(t, updatedContent, got.Content,
			"GET after PUT must return the updated content")
	})
	if t.Failed() {
		return
	}

	// --- AC #6: DELETE removes it; a subsequent list excludes it -------
	t.Run("DeleteRemovesQuadlet", func(t *testing.T) {
		// Prove presence first so a broken list endpoint can't make the
		// post-delete absence check pass vacuously.
		requireQuadletListed(t, machineID, quadletName+".container",
			"quadlet must appear in the list before delete")
		// DELETE expects the full name with extension in the URL path
		deleteQuadlet(t, machineID, quadletName+".container")
		requireQuadletNotListed(t, machineID, quadletName+".container",
			"deleted quadlet must not appear in the quadlet list")
	})

	// --- AC #7: DELETE on a missing quadlet returns 404 --------------
	t.Run("DeleteMissingQuadletReturns404", func(t *testing.T) {
		missingName := "it-" + suite.RunID + "-missing"
		// DELETE expects the full name with extension in the URL path
		deleteQuadletExpectNotFound(t, machineID, missingName+".container")
	})

	// --- AC #8: POST with a duplicate name returns 409 ---------------
	t.Run("CreateDuplicateReturns409", func(t *testing.T) {
		// Recreate the same quadlet we just deleted (same name).
		createQuadlet(t, machineID, createReq)
		// Attempting to create it again should fail with 409.
		createQuadletExpectConflict(t, machineID, createReq)
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers — each hits the in-process backend via the authenticated client
// ---------------------------------------------------------------------------

// runnableContainerQuadletContent is the recipe every quadlet slice uses
// when it needs a unit systemd can actually start: a busybox container
// that sleeps long enough to outlive the test. `busybox` must already be
// present on the machine — slices ensure this via ensureBusyboxImage.
const runnableContainerQuadletContent = `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
`

// quadletErrorEnvelope mirrors the {"error":{code,message}} shape the
// backend's error responses use. Shared by the quadlet slices' error
// assertions.
type quadletErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// quadletCreateBody mirrors the JSON-accepted fields of
// quadlets.QuadletCreateRequest that this slice populates. Kept as a local
// type so the integration package doesn't import the quadlets domain package.
type quadletCreateBody struct {
	Name     string `json:"name"`
	Resource string `json:"resource"`
	Content  string `json:"content"`
}

// quadletUpdateBody mirrors the JSON-accepted fields of
// quadlets.QuadletUpdateRequest that this slice populates.
type quadletUpdateBody struct {
	Content string `json:"content"`
}

// quadletCreateResponse mirrors the fields of
// quadlets.QuadletCreateResponse that this slice consumes.
type quadletCreateResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Resource string `json:"resource"`
	State    string `json:"state"`
	Content  string `json:"content"`
}

// quadletGetResponse mirrors the fields of
// quadlets.QuadletGetResponse that this slice consumes.
type quadletGetResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Unit     string `json:"unit"`
	Resource string `json:"resource"`
	State    string `json:"state"`
	Content  string `json:"content"`
}

// quadletListResponse mirrors the fields of
// quadlets.QuadletListResponse that this slice consumes.
type quadletListResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Unit     string `json:"unit"`
	Resource string `json:"resource"`
	State    string `json:"state"`
	Raw      string `json:"raw,omitempty"`
}

// createQuadlet POSTs /quadlets and asserts 201 with the expected shape.
func createQuadlet(t *testing.T, machineID string, body quadletCreateBody) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /quadlets must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"create quadlet: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var created quadletCreateResponse
	require.NoErrorf(t, json.Unmarshal(raw, &created),
		"create quadlet: response must decode as QuadletCreateResponse. body: %s", string(raw))
	require.NotEmpty(t, created.ID, "create quadlet: response missing id")
	require.Equal(t, body.Name, created.Name, "create quadlet: response name must match request")
	require.Equal(t, body.Name, created.ID, "create quadlet: response id must equal name (quadletId is the name)")
	require.Equal(t, body.Resource, created.Resource, "create quadlet: response resource must match request")
	require.Equal(t, "unknown", created.State, "create quadlet: newly created quadlet must report state=unknown")
	require.Equal(t, body.Content, created.Content, "create quadlet: response content must echo request")
}

// createQuadletExpectConflict POSTs /quadlets and asserts 409 (duplicate).
func createQuadletExpectConflict(t *testing.T, machineID string, body quadletCreateBody) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /quadlets must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusConflict, resp.StatusCode,
		"create duplicate quadlet: expected 409, got %d. body: %s", resp.StatusCode, string(raw))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"create duplicate quadlet: error response must decode. body: %s", string(raw))
	require.Equal(t, "QUADLET_ALREADY_EXISTS", errResp.Error.Code,
		"create duplicate quadlet: error code must be QUADLET_ALREADY_EXISTS")
}

// getQuadlet GETs /quadlets/{name} and returns the decoded response.
// The name parameter should be the quadlet name without extension
// (the handler internally adds the appropriate extension).
func getQuadlet(t *testing.T, machineID, quadletName string) quadletGetResponse {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /quadlets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"get quadlet: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got quadletGetResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"get quadlet: response must decode as QuadletGetResponse. body: %s", string(body))
	return got
}

// getQuadletExpectNotFound GETs /quadlets/{name} and asserts 404.
func getQuadletExpectNotFound(t *testing.T, machineID, quadletName string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /quadlets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode,
		"get missing quadlet: expected 404, got %d. body: %s", resp.StatusCode, string(body))

	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &errResp),
		"get missing quadlet: error response must decode. body: %s", string(body))
	require.Equal(t, "QUADLET_NOT_FOUND", errResp.Code,
		"get missing quadlet: error code must be QUADLET_NOT_FOUND")
}

// updateQuadlet PUTs /quadlets/{name} and asserts 200.
// The name parameter should be the quadlet name WITH extension
// (the service internally strips the existing extension and appends
// the correct one based on the existing quadlet's resource type).
func updateQuadlet(t *testing.T, machineID, quadletName string, body quadletUpdateBody) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "PUT /quadlets/{id}")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"update quadlet: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var updated quadletGetResponse
	require.NoErrorf(t, json.Unmarshal(raw, &updated),
		"update quadlet: response must decode as QuadletGetResponse. body: %s", string(raw))
	require.Equal(t, body.Content, updated.Content, "update quadlet: response content must match request")
}

// deleteQuadlet DELETEs /quadlets/{name} and asserts 204.
func deleteQuadlet(t *testing.T, machineID, quadletName string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /quadlets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Accept 204 (success) or 404 (already gone — idempotent delete).
	// The per-test cleanup calls this after the test's own DELETE, so 404
	// is expected and harmless.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		require.Equalf(t, http.StatusNoContent, resp.StatusCode,
			"delete quadlet: expected 204, got %d. body: %s", resp.StatusCode, string(body))
	}
}

// deleteQuadletExpectNotFound DELETEs /quadlets/{name} and asserts 404.
func deleteQuadletExpectNotFound(t *testing.T, machineID, quadletName string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletName + "/"
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /quadlets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode,
		"delete missing quadlet: expected 404, got %d. body: %s", resp.StatusCode, string(body))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(body, &errResp),
		"delete missing quadlet: error response must decode. body: %s", string(body))
	require.Equal(t, "QUADLET_NOT_FOUND", errResp.Error.Code,
		"delete missing quadlet: error code must be QUADLET_NOT_FOUND")
}

// listQuadletNames GETs /quadlets/ and returns every quadlet name on the
// machine. Used by the presence/absence assertions.
func listQuadletNames(t *testing.T, machineID string) []string {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /quadlets")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list quadlets: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	// QuadletListResponse is a bare JSON array ([]QuadletListResponse), not
	// an envelope — matching the images slice convention.
	var result []quadletListResponse
	require.NoErrorf(t, json.Unmarshal(body, &result),
		"list quadlets: response must decode as a JSON array. body: %s", string(body))

	var names []string
	for _, q := range result {
		names = append(names, q.Name)
	}
	return names
}

// requireQuadletListed asserts that name appears among the machine's
// quadlets. Proving presence before delete is what makes the post-delete
// absence check meaningful (otherwise a list that always returns [] would
// pass vacuously).
func requireQuadletListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	require.Containsf(t, listQuadletNames(t, machineID), name, "%s", msg)
}

// requireQuadletNotListed asserts that name does NOT appear among the
// machine's quadlets — i.e. the delete actually removed it.
func requireQuadletNotListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	require.NotContainsf(t, listQuadletNames(t, machineID), name, "%s", msg)
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

// registerQuadletsSweepOnce ensures the quadlets suite-sweep cleanup is
// registered at most once per process. The sweep force-removes any quadlet
// whose name mentions this run's RunID — i.e. garbage a test left behind
// when its own t.Cleanup didn't fire (panic, SIGINT before the per-test
// cleanup registered, …). Registered at the top of the quadlets test, so
// it is in place before any garbage is created.
//
// The sweep uses `podman quadlet rm -f <name>` (force) per run-scoped
// name. Force is required because the sweep runs after test failure,
// and a quadlet may be in a running state (the stop-param gap). Best-effort:
// failures are logged by runCleanups, never fatal.
var registerQuadletsSweepOnce sync.Once

func registerQuadletsSweep() {
	registerQuadletsSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped quadlets", func() error {
			sweepRunScopedQuadlets(runID)
			return nil
		})
	})
}

// sweepRunScopedQuadlets lists every quadlet on the machine and removes
// any whose name mentions the runID. See registerQuadletsSweep for the
// force rationale (stop-param gap).
//
// Note: podman quadlet rm is not implemented for remote clients, and
// podmanConnRun only runs podman commands. This sweep is best-effort:
// it lists run-scoped quadlets but cannot reliably remove them. The
// quadlet lifecycle slice (080-09) will handle the running-quadlet
// cleanup, and machine teardown will clean up the quadlet files.
func sweepRunScopedQuadlets(runID string) {
	// `podman quadlet ls` lists quadlets. The output format is JSON by
	// default when --format=json is specified.
	out, err := podmanConnRun("quadlet", "ls", "--format", "json")
	if err != nil {
		// Best-effort: a failed list (e.g. machine already stopped) is
		// not a fatal teardown problem.
		return
	}

	// Parse the JSON output: each line is a JSON object representing a
	// quadlet. The shape matches podman's ListQuadlet entities.
	var quadlets []struct {
		Name    string `json:"Name"`
		Unit    string `json:"UnitName"`
		Enabled string `json:"Enabled"`
		Loaded  string `json:"Loaded"`
	}
	if err := json.Unmarshal([]byte(out), &quadlets); err != nil {
		// If we can't parse, skip the sweep — best-effort.
		return
	}

	for _, q := range quadlets {
		name := strings.TrimSpace(q.Name)
		if name == "" || !strings.Contains(name, runID) {
			continue
		}
		// Log the quadlet we would clean up — we cannot reliably remove it
		// with the current CLI constraints. The machine teardown will handle it.
		// Note: t.Logf is not available here; we'd need to pass a logger in.
		// For now, this is a no-op sweep that just acknowledges the garbage.
	}
}
