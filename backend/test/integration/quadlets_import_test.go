//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_Import is the quadlet import vertical slice for
// the real-podman integration suite (PRD 080-12). It proves the bulk-install
// + partial-failure path of the content-based import endpoint:
//
//	POST /api/machines/{id}/quadlets/import
//	     body {quadlets: [{name, resource, content}]} →
//	     201 {imported: [{id, name, path, state}], failed: [{name, error}]}
//
// Each entry is tar-wrapped by the service and installed via podman's
// QuadletInstall over the SSH tunnel, one entry at a time. Failures are
// collected per-entry rather than failing the whole request — so a name
// collision on one entry must not prevent its valid siblings from importing.
//
// Isolation/cleanup model (matching the CRUD slice, 080-08): every quadlet
// this test creates — the imported entries plus the pre-seeded collision
// target — carries a run-scoped name (it-<runID>-…) and gets a t.Cleanup
// DELETE via the API. registerQuadletsSweep is the abort backstop for
// anything the per-test cleanup leaves behind.
func Test_Integration_Quadlets_Import(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// The .container recipe reuses the shared runnable quadlet content.
	// Install itself never starts the container, but keeping the recipe
	// uniform means the imported units are startable by later slices.
	ensureBusyboxImage(t)

	impA := "it-" + suite.RunID + "-imp-a"
	impB := "it-" + suite.RunID + "-imp-b"
	impC := "it-" + suite.RunID + "-imp-c"
	impD := "it-" + suite.RunID + "-imp-d"
	collide := "it-" + suite.RunID + "-collide"

	// Per-test cleanup for everything this test installs or seeds.
	// Registered before the creates so it runs even if a later phase
	// fails. deleteQuadlet accepts 404 (already gone) — idempotent.
	for _, name := range []string{impA, impB, impC, impD, collide} {
		t.Cleanup(func() {
			deleteQuadlet(t, machineID, name+".container")
		})
	}

	// Seed the collision target: a pre-existing quadlet named `collide`.
	// Parent-level setup like the generate slice's source container —
	// createQuadlet's asserts are fatal, so no t.Failed guard is needed.
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     collide,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})

	// --- AC #1: import multiple valid entries → 201, all in imported[] -
	t.Run("AllValidEntriesImport", func(t *testing.T) {
		imported, failed := importQuadlets(t, machineID, quadletImportBody{
			Quadlets: []quadletImportEntry{
				{Name: impA, Resource: "container", Content: runnableContainerQuadletContent},
				{Name: impB, Resource: "container", Content: runnableContainerQuadletContent},
			},
		})

		require.Empty(t, failed, "no entry may fail when all entries are valid")
		require.Len(t, imported, 2, "both valid entries must be reported in imported[]")

		names := make([]string, 0, len(imported))
		for _, result := range imported {
			require.Equal(t, result.Name, result.ID,
				"imported entry id must equal the requested name")
			require.Equalf(t, "unknown", result.State,
				"imported entry %s: freshly imported quadlet must report state=unknown", result.Name)
			names = append(names, result.Name)
		}
		require.ElementsMatch(t, []string{impA, impB}, names,
			"every valid entry must appear in imported[]")
	})
	if t.Failed() {
		return
	}

	// --- AC #2: collision lands in failed[], valid siblings still
	// import (partial success). Two valid entries bracket the colliding
	// one to prove a mid-request failure doesn't abort the batch.
	t.Run("CollisionFailsEntrySiblingsImport", func(t *testing.T) {
		imported, failed := importQuadlets(t, machineID, quadletImportBody{
			Quadlets: []quadletImportEntry{
				{Name: impC, Resource: "container", Content: runnableContainerQuadletContent},
				{Name: collide, Resource: "container", Content: runnableContainerQuadletContent},
				{Name: impD, Resource: "container", Content: runnableContainerQuadletContent},
			},
		})

		require.Len(t, imported, 2,
			"the valid siblings of a failed entry must still import")
		importedNames := make([]string, 0, len(imported))
		for _, result := range imported {
			importedNames = append(importedNames, result.Name)
		}
		require.ElementsMatch(t, []string{impC, impD}, importedNames,
			"imported[] must contain exactly the non-colliding entries")

		require.Len(t, failed, 1,
			"exactly the colliding entry may fail")
		require.Equal(t, collide, failed[0].Name,
			"the failed entry must be reported by name")
		require.NotEmpty(t, failed[0].Error,
			"the failed entry must carry a non-empty error")
		t.Logf("collision error: %s", failed[0].Error)
	})
	if t.Failed() {
		return
	}

	// --- AC #3: imported quadlets visible via GET /quadlets -----------
	t.Run("ImportedQuadletsAreListed", func(t *testing.T) {
		for _, name := range []string{impA, impB, impC, impD} {
			requireQuadletListed(t, machineID, name+".container",
				"imported quadlet must appear in the quadlet list (podman reports names with extension)")
		}

		// The collision must not have clobbered the pre-existing
		// quadlet: the failed entry was rejected, so its content is
		// still the seeded recipe.
		got := getQuadlet(t, machineID, collide+".container")
		require.Equal(t, runnableContainerQuadletContent, got.Content,
			"a failed import entry must not overwrite the existing quadlet")
	})

	// --- AC #4: VALIDATION_FAILED (400) on malformed requests ---------
	t.Run("EmptyEntriesReturns400", func(t *testing.T) {
		importQuadletsExpectError(t, machineID, quadletImportBody{
			Quadlets: []quadletImportEntry{},
		}, http.StatusBadRequest, "VALIDATION_FAILED",
			"import with no entries")
	})

	t.Run("InvalidResourceReturns400", func(t *testing.T) {
		importQuadletsExpectError(t, machineID, quadletImportBody{
			Quadlets: []quadletImportEntry{
				{Name: "it-" + suite.RunID + "-badres", Resource: "instalment", Content: runnableContainerQuadletContent},
			},
		}, http.StatusBadRequest, "VALIDATION_FAILED",
			"import with an invalid resource")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers — each hits the in-process backend via the authenticated client
// ---------------------------------------------------------------------------

// quadletImportEntry mirrors quadlets.QuadletImportEntry that this slice
// populates. Kept as a local type so the integration package doesn't import
// the quadlets domain package (matching the other quadlet slices).
type quadletImportEntry struct {
	Name     string `json:"name"`
	Resource string `json:"resource"`
	Content  string `json:"content"`
}

// quadletImportBody mirrors the JSON-accepted fields of
// quadlets.QuadletImportRequest.
type quadletImportBody struct {
	Quadlets []quadletImportEntry `json:"quadlets"`
}

// quadletImportResult mirrors quadlets.QuadletImportResult.
type quadletImportResult struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
}

// quadletImportError mirrors quadlets.QuadletImportError.
type quadletImportError struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// importQuadlets POSTs /quadlets/import, asserts 201, and decodes the
// {imported, failed} response.
func importQuadlets(t *testing.T, machineID string, body quadletImportBody) (imported []quadletImportResult, failed []quadletImportError) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/import"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /quadlets/import must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"import quadlets: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var result struct {
		Imported []quadletImportResult `json:"imported"`
		Failed   []quadletImportError  `json:"failed"`
	}
	require.NoErrorf(t, json.Unmarshal(raw, &result),
		"import quadlets: response must decode as QuadletImportResponse. body: %s", string(raw))
	return result.Imported, result.Failed
}

// importQuadletsExpectError POSTs /quadlets/import and asserts the expected
// HTTP status plus the machine-readable error code from the
// {"error":{code,message}} envelope.
func importQuadletsExpectError(t *testing.T, machineID string, body quadletImportBody, wantStatus int, wantCode, context string) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/import"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoErrorf(t, err, "%s: POST /quadlets/import must connect", context)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"%s: expected %d, got %d. body: %s", context, wantStatus, resp.StatusCode, string(raw))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", context, string(raw))
	require.Equal(t, wantCode, errResp.Error.Code,
		"%s: error code must be %s", context, wantCode)
}
