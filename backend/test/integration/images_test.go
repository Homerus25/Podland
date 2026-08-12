//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Images_PullListDelete is the images vertical slice for
// the real-podman integration suite (PRD 080-02). It exercises the full
// images pipeline end-to-end against the once-registered machine:
//
//	POST   /api/machines/{id}/images/pull   → NDJSON progress stream
//	GET    /api/machines/{id}/images        → list (run-scoped filter)
//	DELETE /api/machines/{id}/images/{id}   → remove
//
// The pull proves the registry → SSH tunnel → podman socket path,
// including consuming the NDJSON pull-progress stream. List and delete
// then operate on exactly what the pull produced, scoped by a run-scoped
// tag so a pre-dirty shared machine can't cause a false failure.
//
// Isolation model (per the master PRD's "run-scoped tag + per-test
// cleanup + suite sweep" ruling):
//
//   - The pulled image is tagged with a run-scoped reference
//     (localhost/it-<runID>-busybox:latest) so list/count assertions and
//     cleanup can find exactly what this run created. Tagging is done
//     over the same podman connection the backend uses — it is test
//     scaffolding for isolation, NOT part of the system under test (the
//     API has no tag endpoint today).
//   - t.Cleanup fully restores state: it removes the run-scoped tag and,
//     if busybox:latest already lived on the machine before this run,
//     re-pulls it so the shared machine is left as found.
//   - A suite-teardown sweep (registered once per process) removes any
//     leftover run-scoped references if a test aborted before its
//     t.Cleanup fired (e.g. SIGINT).
func Test_Integration_Images_PullListDelete(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerImagesSweep()

	machineID := suite.MachineID
	runScopedRef := "localhost/it-" + suite.RunID + "-busybox:latest"
	// runScopedNeedle is the substring used for client-side filtering of
	// RepoTags. It intentionally omits the ":latest" suffix and the
	// "localhost/" prefix-length dependency so it matches regardless of
	// how podman normalises the stored reference.
	runScopedNeedle := "it-" + suite.RunID + "-busybox"

	// Detect whether busybox already lived on the shared machine so
	// cleanup can restore it. The needle is the canonical fully-qualified
	// repo that `podman pull busybox:latest` produces — NOT a bare
	// "busybox" substring — so unrelated images (e.g. my-busybox-tool)
	// can't false-positive into an unnecessary (and junk-leaving)
	// restore re-pull.
	busyboxPreexisted := imageRepoPresent(t, machineID, "docker.io/library/busybox")
	t.Logf("pre-check: busybox pre-existing on machine = %v", busyboxPreexisted)

	// Per-test cleanup. Registered before the pull so it runs even if a
	// later phase fails. Restores the shared machine to its prior state.
	t.Cleanup(func() {
		// Remove the run-scoped tag we created. Best-effort: errors are
		// logged, not fatal (the sweep is the backstop).
		if out, err := podmanConnRun("rmi", runScopedRef); err != nil {
			t.Logf("cleanup: rmi %q best-effort failed: %v (%s)", runScopedRef, err, strings.TrimSpace(out))
		}
		// If busybox:latest was on the machine before we touched it, the
		// force-delete-by-ID below removed it along with our tag. Pull it
		// back so we leave no trace. This is the only path that mutates
		// pre-existing state, and it restores it exactly.
		if busyboxPreexisted {
			if out, err := podmanConnRun("pull", "busybox:latest"); err != nil {
				t.Logf("cleanup: restore busybox:latest best-effort failed: %v (%s)", err, strings.TrimSpace(out))
			}
		}
	})

	// --- AC #1: pull via API, consume NDJSON progress to completion ---
	t.Run("PullBusybox", func(t *testing.T) {
		ok := pullImageStreamToCompletion(t, machineID, "busybox:latest")
		require.True(t, ok,
			"pull must terminate with a 'complete' event and no 'error' event")
	})
	// Fail fast: the tag/list/delete phases all depend on a successful
	// pull. Returning here still runs t.Cleanup (registered above) and
	// the suite sweep — they are best-effort no-ops on a missing image.
	if t.Failed() {
		return
	}

	// Isolation scaffolding: tag the pulled image run-scoped. Done over
	// the same podman connection the backend uses; not the SUT.
	out, err := podmanConnRun("tag", "busybox:latest", runScopedRef)
	require.NoError(t, err,
		"tag pulled image with run-scoped reference for isolation: %s", strings.TrimSpace(out))

	// --- AC #2: list returns the pulled image (run-scoped assertion) ---
	var imageID string
	t.Run("ListReturnsPulledImage", func(t *testing.T) {
		imageID = findRunScopedImageID(t, machineID, runScopedNeedle)
		require.NotEmpty(t, imageID, "run-scoped image must expose an ID for delete")
		t.Logf("run-scoped image found: id=%s", imageID)
	})
	if t.Failed() {
		return
	}

	// --- AC #3: delete removes it; subsequent list no longer has it ---
	t.Run("DeleteRemovesImage", func(t *testing.T) {
		require.NotEmpty(t, imageID, "delete requires the image ID captured in the list phase")
		deleteImageByID(t, machineID, imageID)
		requireNoRunScopedImage(t, machineID, runScopedNeedle)
	})
}

// ---------------------------------------------------------------------------
// Pull: consume the NDJSON progress stream
// ---------------------------------------------------------------------------

// pullStreamTimeout caps the whole pull HTTP exchange. The handler writes
// a terminal "complete"/"error" event and returns, which closes the body
// and ends the stream normally; the timeout is purely a hang guard in
// case the server stops sending without closing (no observed case).
const pullStreamTimeout = 3 * time.Minute

// pullImageStreamToCompletion POSTs /images/pull and reads the NDJSON
// progress stream until the server closes the body. It returns true iff
// the stream emitted a terminal "complete" event and no "error" event.
//
// A dedicated *http.Client with no Timeout is used because http.Client's
// Timeout covers the entire request including body read — which would cut
// a streaming pull mid-stream. The hang guard is the request-scoped
// context (pullStreamTimeout) instead.
func pullImageStreamToCompletion(t *testing.T, machineID, image string) bool {
	t.Helper()
	body, err := json.Marshal(map[string]string{"image": image})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), pullStreamTimeout)
	defer cancel()

	pullURL := suite.ServerURL + "/api/machines/" + machineID + "/images/pull"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pullURL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// Bearer header is set by hand (not via suite.AuthenticatedClient)
	// because we need a no-timeout client for streaming. main_test.go's
	// registerMachine follows the same hand-set-header pattern when it
	// also can't use the default Suite client.
	req.Header.Set("Authorization", "Bearer "+suite.Token)

	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err, "POST /images/pull must connect to the in-process server")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"pull must return 200 to begin the NDJSON stream. status=%d", resp.StatusCode)

	// Each line is one PullProgressEvent (see images/dto.go). We only
	// need the `type` discriminator to detect stream termination.
	scanner := bufio.NewScanner(resp.Body)
	var (
		sawComplete bool
		sawError    bool
		eventCount  int
		lastType    string
	)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		eventCount++
		var ev struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Don't fail the whole test on one malformed line — log and
			// keep draining so a later terminal event still lands.
			t.Logf("pull: event #%d unparseable, continuing: %v (line=%s)", eventCount, err, line)
			continue
		}
		lastType = ev.Type
		switch ev.Type {
		case "complete":
			sawComplete = true
		case "error":
			sawError = true
			t.Logf("pull: error event received: %s", line)
		}
	}
	require.NoError(t, scanner.Err(), "reading pull NDJSON stream")
	require.Greater(t, eventCount, 0, "pull stream must emit at least one event")
	t.Logf("pull stream consumed: %d events, last type=%q (complete=%v error=%v)",
		eventCount, lastType, sawComplete, sawError)
	return sawComplete && !sawError
}

// ---------------------------------------------------------------------------
// List: GET /images and run-scoped client-side filtering
// ---------------------------------------------------------------------------

// imageDTO mirrors the fields of images.ImageListResponse that this slice
// consumes. Kept as a local type so the integration package doesn't import
// the images domain package (matching the tracer test's style of decoding
// into local structs).
type imageDTO struct {
	ID       string   `json:"id"`
	RepoTags []string `json:"repoTags"`
}

// listImages GETs /api/machines/{id}/images/ and decodes the JSON array.
// The list endpoint returns a bare JSON array ([]ImageListResponse), not
// an envelope — unlike the containers list which wraps in an object.
func listImages(t *testing.T, machineID string) []imageDTO {
	t.Helper()
	listURL := suite.ServerURL + "/api/machines/" + machineID + "/images/"
	resp, err := suite.AuthenticatedClient().Get(listURL)
	require.NoError(t, err, "GET /images")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read images response body")
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"list images: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var out []imageDTO
	require.NoError(t, json.Unmarshal(body, &out),
		"list images: response must decode as a JSON array. body: %s", string(body))
	return out
}

// tagsContainNeedle reports whether any entry of tags contains needle.
// Case-sensitive: repo tags are conventionally lower-case and the
// run-scoped needle is an exact hex runID, so case folding would only
// mask a real mismatch.
func tagsContainNeedle(tags []string, needle string) bool {
	for _, tag := range tags {
		if strings.Contains(tag, needle) {
			return true
		}
	}
	return false
}

// imageRepoPresent reports whether any image on the machine carries a
// repo tag containing needle. Client-side check over an unfiltered list
// so the result is independent of podman's reference-filter quirks.
func imageRepoPresent(t *testing.T, machineID, needle string) bool {
	t.Helper()
	for _, img := range listImages(t, machineID) {
		if tagsContainNeedle(img.RepoTags, needle) {
			return true
		}
	}
	return false
}

// findRunScopedImageID lists images and asserts that exactly one carries
// a tag mentioning the run-scoped needle, returning its image ID. The
// assertion is scoped to the run so other images on the shared machine
// cannot cause a false failure (PRD AC #2).
func findRunScopedImageID(t *testing.T, machineID, needle string) string {
	t.Helper()
	var matched []imageDTO
	for _, img := range listImages(t, machineID) {
		if tagsContainNeedle(img.RepoTags, needle) {
			matched = append(matched, img)
		}
	}
	require.Lenf(t, matched, 1,
		"expected exactly one run-scoped image matching %q, got %d: %+v",
		needle, len(matched), matched)
	return matched[0].ID
}

// requireNoRunScopedImage asserts that no image on the machine carries a
// run-scoped tag anymore — i.e. the delete actually removed it.
func requireNoRunScopedImage(t *testing.T, machineID, needle string) {
	t.Helper()
	var leftover []imageDTO
	for _, img := range listImages(t, machineID) {
		if tagsContainNeedle(img.RepoTags, needle) {
			leftover = append(leftover, img)
		}
	}
	require.Emptyf(t, leftover,
		"run-scoped image still present after delete (needle=%q): %+v", needle, leftover)
}

// ---------------------------------------------------------------------------
// Delete: DELETE /images/{imageId}
// ---------------------------------------------------------------------------

// deleteImageByID issues DELETE /images/{imageId}?force=true. Force is
// required because the run-scoped tag step leaves the image with two
// repo tags (busybox:latest + the run-scoped one); without force podman
// refuses to remove an image with multiple dependent tags.
//
// force=true removes the image and ALL its tags, so if busybox:latest
// pre-existed it is also removed here — t.Cleanup re-pulls it to restore
// the prior state.
func deleteImageByID(t *testing.T, machineID, imageID string) {
	t.Helper()
	delURL := suite.ServerURL + "/api/machines/" + machineID + "/images/" + imageID + "?force=true"
	req, err := http.NewRequest(http.MethodDelete, delURL, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /images/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusNoContent, resp.StatusCode,
		"delete image: expected 204, got %d. body: %s", resp.StatusCode, string(body))
}

// ---------------------------------------------------------------------------
// CLI scaffolding (isolation + cleanup)
// ---------------------------------------------------------------------------

// podmanCLITimeout caps each isolation/cleanup CLI call. A hung podman
// invocation must not hang the whole suite (teardown included). Generous
// enough for the restore re-pull (registry fetch over the SSH tunnel).
const podmanCLITimeout = 2 * time.Minute

// podmanConnRun runs `podman --connection <MachineName> <args...>` with
// a bounded context and returns the combined output. Used for the
// isolation/cleanup CLI ops the images API doesn't expose (tag) or that
// must work after the backend has torn down (sweep).
//
// The connection name comes from Suite.MachineName, which TestMain
// resolves and whose reachability the readiness probe guarantees — so
// this hits the same host the backend does, including the SSH-override
// escape hatch (as long as a matching podman connection exists).
func podmanConnRun(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), podmanCLITimeout)
	defer cancel()
	full := append([]string{"--connection", suite.MachineName}, args...)
	out, err := exec.CommandContext(ctx, "podman", full...).CombinedOutput()
	if err != nil {
		// Match main_test.go's convention of folding podman's stderr
		// into the error inline, rather than a custom error type.
		if trim := strings.TrimSpace(string(out)); trim != "" {
			return string(out), fmt.Errorf("%w (%s)", err, trim)
		}
	}
	return string(out), err
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

// registerSweepOnce ensures the images suite-sweep cleanup is registered
// at most once per process. The sweep removes any image whose repo tag
// mentions this run's RunID — i.e. garbage a test left behind when its
// own t.Cleanup didn't fire (panic, SIGINT before the per-test cleanup
// registered, etc.). Registered at the top of the images test, so it is
// in place before any garbage is created.
//
// The sweep uses `podman rmi <ref>` (no force) per run-scoped reference:
// that removes just our tag if the image is still shared with another
// tag, and removes the image only if our tag was the last one. It is
// best-effort — failures are logged by runCleanups, never fatal.
var registerSweepOnce sync.Once

func registerImagesSweep() {
	registerSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped images", func() error {
			sweepRunScopedImages(runID)
			return nil
		})
	})
}

// sweepRunScopedImages lists every image on the machine and removes any
// whose repo tag mentions the runID. See registerImagesSweep for the
// non-destructive (no-force) rationale.
func sweepRunScopedImages(runID string) {
	out, err := podmanConnRun("images", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		// Best-effort: a failed list (e.g. machine already stopped) is
		// not a fatal teardown problem.
		return
	}
	for _, line := range strings.Split(out, "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" || !strings.Contains(ref, runID) {
			continue
		}
		// rmi the run-scoped reference. Ignore errors: the per-test
		// cleanup is the primary path; this is the backstop.
		_, _ = podmanConnRun("rmi", ref)
	}
}
