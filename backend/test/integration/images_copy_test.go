//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cross-host image copy slice (issue 375): the images domain's
// composition over the transfer kit's image splice. A copy exports the
// source image's archive on host A and loads it on host B — the
// load is content-addressed, so a second copy of an already-present
// image succeeds without re-transferring (idempotence), and the preview
// reports the dialog's one fact: whether the target already holds the
// image by ID.
//
// Everything here is API-driven on both hosts: the image is ensured
// on host A (the containers slice's busybox fixture), copied through
// the in-process server's NDJSON stream, and verified present on host
// B through the images list API — so the slice runs wherever the
// two-host harness runs. No CLI calls against host B at all: the
// copy endpoint and the list API observe B's state entirely.

// imageCopyEvent mirrors one NDJSON event of an image copy; only the
// fields this slice asserts are named.
type imageCopyEvent struct {
	OperationID string `json:"operationId"`
	Type        string `json:"type"`
	HostID      string `json:"hostId"`
	Image       string `json:"image,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Result      *struct {
		ID           string   `json:"id"`
		Status       string   `json:"status"`
		SourceHostID string   `json:"sourceHostId"`
		TargetHostID string   `json:"targetHostId"`
		Bytes        int64    `json:"bytes"`
		Names        []string `json:"names"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// imageCopyPreviewResponse mirrors the preview's JSON shape; only the
// fields this slice asserts are named.
type imageCopyPreviewResponse struct {
	Present bool `json:"present"`
}

// parseImageCopyEvents decodes the streamed NDJSON body into events.
func parseImageCopyEvents(t *testing.T, body string) []imageCopyEvent {
	t.Helper()
	var events []imageCopyEvent
	scanner := bufio.NewScanner(bytes.NewReader([]byte(body)))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event imageCopyEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must decode: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// postImageCopy posts a copy body to the image copy endpoint and
// returns the response's status and body (the stream answers 200 with
// an NDJSON body even when the operation fails — refusals ride the
// terminal event).
func postImageCopy(t *testing.T, hostID, imageID, targetHostID string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"targetHostId": targetHostID})
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/hosts/"+hostID+"/images/"+imageID+"/copy",
		"application/json", bytes.NewReader(body))
	require.NoError(t, err, "POST /images/{id}/copy must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// postImageCopyEvents posts a copy and returns the parsed event stream,
// asserting the stream itself answered 200.
func postImageCopyEvents(t *testing.T, hostID, imageID, targetHostID string) []imageCopyEvent {
	t.Helper()
	status, raw := postImageCopy(t, hostID, imageID, targetHostID)
	require.Equalf(t, http.StatusOK, status,
		"the image copy's stream itself must succeed. body: %s", string(raw))
	return parseImageCopyEvents(t, string(raw))
}

// getImageCopyPreview GETs the copy preview and returns the decoded
// response.
func getImageCopyPreview(t *testing.T, hostID, imageID, targetHostID string) imageCopyPreviewResponse {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/hosts/" + hostID + "/images/" + imageID +
			"/copy-preview?targetHostId=" + targetHostID)
	require.NoError(t, err, "GET copy-preview must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"copy preview: expected 200, got %d. body: %s", resp.StatusCode, string(raw))
	var got imageCopyPreviewResponse
	require.NoErrorf(t, json.Unmarshal(raw, &got),
		"copy preview must decode. body: %s", string(raw))
	return got
}

// findBusyboxImageID lists images on the host and returns the ID of
// the one carrying the exact busybox:latest tag. A tag points at exactly
// one image, so the match is unique by construction; the require guards
// against a fixture drift (ensureBusyboxImage not called, tag renamed).
func findBusyboxImageID(t *testing.T, hostID string) string {
	t.Helper()
	var matched []imageDTO
	for _, img := range listImages(t, hostID) {
		for _, tag := range img.RepoTags {
			if tag == "busybox:latest" {
				matched = append(matched, img)
				break
			}
		}
	}
	require.Lenf(t, matched, 1,
		"expected exactly one image tagged busybox:latest, got %d: %+v",
		len(matched), matched)
	return matched[0].ID
}

// imageIDPresent reports whether any image in the list carries the
// given full image ID — the content-addressed proof that the copy
// landed: the loaded image on host B must have the same ID as the
// source on host A.
func imageIDPresent(images []imageDTO, imageID string) bool {
	for _, img := range images {
		if img.ID == imageID {
			return true
		}
	}
	return false
}

// cleanupImageOnB registers a best-effort API deletion of the copied
// image on host B — the fixture discipline for this slice's
// target-side creations. Best-effort mirrors cleanupVolume: a failed
// teardown (host already stopped, image already gone) must not fail
// the test.
func cleanupImageOnB(t *testing.T, hostID, imageID string) {
	t.Helper()
	t.Cleanup(func() {
		req := standardRequest(t, http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+hostID+"/images/"+imageID+"?force=true")
		resp, err := suite.AuthenticatedClient().Do(req)
		if err != nil {
			t.Logf("cleanup image %s on %s: %v", imageID, hostID, err)
			return
		}
		resp.Body.Close()
	})
}

// Test_Integration_Images_Copy pins the cross-host image copy
// contract end to end: a copy carries the image to host B (same
// content-addressed ID), the preview flips from absent to present, and
// a second copy of the now-present image succeeds — the load's
// idempotence, no false collision.
func Test_Integration_Images_Copy(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerImagesSweep()

	b := suite.RegisterSecondHost(t)
	if !b.Independent {
		t.Skip("host B shares host A's host (PODLAND_TEST_SSH_* fallback) — image copies need independent podman state")
	}

	hostA := suite.HostID

	// Fixture: busybox on host A (the containers slice's shared
	// fixture — pulled if missing, left in place afterwards), resolved
	// to its full image ID: the copy endpoint and the preview's
	// present-by-ID fact both speak IDs.
	ensureBusyboxImage(t)
	imageID := findBusyboxImageID(t, hostA)
	cleanupImageOnB(t, b.HostID, imageID)

	// --- AC: the preview reports the target's fact ---------------------
	// Host B is a freshly booted host with an empty store, so the
	// image is absent there before the copy.
	before := getImageCopyPreview(t, hostA, imageID, b.HostID)
	assert.False(t, before.Present,
		"host B boots with an empty image store — the preview must report absent")

	// --- AC: a copy carries the image across hosts ------------------
	events := postImageCopyEvents(t, hostA, imageID, b.HostID)

	// The event stream: image start, image done, one terminal complete
	// carrying the copy envelope (byte-progress events may ride between
	// start and done — the splice emits them live).
	require.GreaterOrEqual(t, len(events), 3)
	assert.Equal(t, "image", events[0].Type)
	assert.Equal(t, "start", events[0].Phase)
	assert.Equal(t, imageID, events[0].Image)
	assert.Equal(t, hostA, events[0].HostID)
	assert.NotEmpty(t, events[0].OperationID, "the transport stamps its operation ID")

	done := events[len(events)-2]
	assert.Equal(t, "image", done.Type)
	assert.Equal(t, "done", done.Phase)

	complete := events[len(events)-1]
	assert.Equal(t, "complete", complete.Type)
	require.NotNil(t, complete.Result)
	assert.Equal(t, "copied", complete.Result.Status)
	assert.Equal(t, imageID, complete.Result.ID)
	assert.Equal(t, hostA, complete.Result.SourceHostID)
	assert.Equal(t, b.HostID, complete.Result.TargetHostID)
	assert.Greater(t, complete.Result.Bytes, int64(0))
	// Names is the load's report; the service guarantees the field is
	// present (a nil report arrives as an empty array, never a missing
	// field).
	require.NotNil(t, complete.Result.Names)

	// The image itself crossed hosts: host B now lists the same
	// content-addressed ID.
	onB := listImages(t, b.HostID)
	assert.Truef(t, imageIDPresent(onB, imageID),
		"host B must list the copied image by ID %s; got %+v", imageID, onB)

	// --- AC: the preview flips once the image is present ---------------
	after := getImageCopyPreview(t, hostA, imageID, b.HostID)
	assert.True(t, after.Present,
		"after a successful copy the preview must report the image present on the target")

	// --- AC: a second copy succeeds — idempotence, no collision -------
	// Images are content-addressed: the load of an already-present image
	// skips its layers and answers success, so the retry is a no-op
	// success — unlike a volume copy, which would refuse with a
	// collision.
	retry := postImageCopyEvents(t, hostA, imageID, b.HostID)
	retryComplete := retry[len(retry)-1]
	assert.Equal(t, "complete", retryComplete.Type)
	require.NotNil(t, retryComplete.Result)
	assert.Equal(t, "copied", retryComplete.Result.Status,
		"a second copy of an already-present image must succeed, not collide")
	assert.Equal(t, imageID, retryComplete.Result.ID)

	// The source keeps its image — a copy never touches the source.
	assert.Truef(t, imageIDPresent(listImages(t, hostA), imageID),
		"a copy must leave the source image on host A")
}
