//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Images_ListFilters is the image list-filter vertical
// slice for the real-podman integration suite. It proves the `filters` query
// parameter of GET /api/machines/{id}/images is applied server-side by
// podman (contract at doku/description_api.md:692-693), through handler →
// service → tunnel ImageEngine.List:
//
//	GET /api/machines/{id}/images?filters={"reference":["<ref>"]}
//
// Fixture: busybox (ensured present) tagged with a run-scoped reference.
// Filtering by that exact reference must return exactly the fixture image,
// while filtering by a reference no image carries must return an empty
// list — that empty result is the tripwire proving the filter actually
// reaches podman (an ignored/never-forwarded filter would return the full
// inventory instead).
//
// Isolation model matches the images slice (080-02): run-scoped tag for
// assertions/cleanup, per-test t.Cleanup, and the shared suite sweep for
// aborts. Tagging is test scaffolding over the same podman connection the
// backend uses, NOT part of the system under test (no tag endpoint exists).
func Test_Integration_Images_ListFilters(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerImagesSweep()

	machineID := suite.MachineID
	runScopedRef := "localhost/it-" + suite.RunID + "-ilf-busybox:latest"
	runScopedNeedle := "it-" + suite.RunID + "-ilf-busybox"

	// Detect whether busybox already lived on the shared machine so
	// cleanup can restore it (same needle discipline as the images slice:
	// the canonical fully-qualified repo, not a bare "busybox" substring).
	busyboxPreexisted := imageRepoPresent(t, machineID, "docker.io/library/busybox")

	// Prerequisite: the fixture needs an image to tag. Idempotent.
	ensureBusyboxImage(t)

	// Per-test cleanup. Registered before the tag so it runs even if a
	// later phase fails. rmi without force removes just our tag when the
	// image is still tagged busybox:latest; if we introduced busybox to
	// the machine ourselves, remove it too to leave no trace.
	t.Cleanup(func() {
		if out, err := podmanConnRun("rmi", runScopedRef); err != nil {
			t.Logf("cleanup: rmi %q best-effort failed: %v (%s)", runScopedRef, err, strings.TrimSpace(out))
		}
		if !busyboxPreexisted {
			if out, err := podmanConnRun("rmi", "busybox:latest"); err != nil {
				t.Logf("cleanup: remove self-pulled busybox:latest best-effort failed: %v (%s)", err, strings.TrimSpace(out))
			}
		}
	})

	// Isolation scaffolding: tag busybox run-scoped (not the SUT).
	out, err := podmanConnRun("tag", "busybox:latest", runScopedRef)
	require.NoError(t, err,
		"tag busybox with run-scoped reference for isolation: %s", strings.TrimSpace(out))

	// Prove the fixture is visible unfiltered first, so the filter
	// assertions below can't pass vacuously through a broken list endpoint.
	var fixtureImageID string
	t.Run("UnfilteredListShowsFixture", func(t *testing.T) {
		fixtureImageID = findRunScopedImageID(t, machineID, runScopedNeedle)
		require.NotEmpty(t, fixtureImageID, "run-scoped fixture must expose an ID")
	})
	if t.Failed() {
		return
	}

	// --- AC #1: reference filter narrows to the matching image --------
	t.Run("ReferenceFilterReturnsOnlyMatchingImage", func(t *testing.T) {
		images := listImagesFiltered(t, machineID, url.Values{
			"filters": {`{"reference":["` + runScopedRef + `"]}`},
		})
		require.Len(t, images, 1,
			"reference filter on the run-scoped ref must return exactly the fixture image, got: %+v", images)
		require.Equal(t, fixtureImageID, images[0].ID,
			"the filtered entry must be the fixture image")
		require.True(t, tagsContainNeedle(images[0].RepoTags, runScopedNeedle),
			"the filtered entry must carry the run-scoped tag, got: %v", images[0].RepoTags)
	})

	// --- AC #2: a reference no image carries yields an empty list -----
	//
	// This is the narrowing tripwire: if the filters param were dropped
	// anywhere along handler → service → podman, the full inventory
	// (including the fixture proven above) would come back instead.
	t.Run("NoMatchReferenceReturnsEmptyList", func(t *testing.T) {
		images := listImagesFiltered(t, machineID, url.Values{
			"filters": {`{"reference":["localhost/it-` + suite.RunID + `-ilf-nosuchimage:latest"]}`},
		})
		require.Empty(t, images,
			"a reference filter matching nothing must return an empty list, got: %+v", images)
	})

	// --- AC #3: filters compose with the all param --------------------
	t.Run("FiltersComposeWithAllParam", func(t *testing.T) {
		images := listImagesFiltered(t, machineID, url.Values{
			"all":     {"true"},
			"filters": {`{"reference":["` + runScopedRef + `"]}`},
		})
		require.Len(t, images, 1,
			"all=true must not disable the reference filter, got: %+v", images)
		require.Equal(t, fixtureImageID, images[0].ID,
			"the filtered entry must still be the fixture image")
	})
}

// listImagesFiltered GETs /images with the given query parameters and
// decodes the JSON array. A nil query lists unfiltered. The response is a
// bare JSON array ([]ImageListResponse) matching listImages.
func listImagesFiltered(t *testing.T, machineID string, query url.Values) []imageDTO {
	t.Helper()

	listURL := suite.ServerURL + "/api/machines/" + machineID + "/images/"
	if len(query) > 0 {
		listURL += "?" + query.Encode()
	}
	resp, err := suite.AuthenticatedClient().Get(listURL)
	require.NoErrorf(t, err, "GET /images?%s must connect to the in-process server", query.Encode())
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read filtered images response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list images?%s: expected 200, got %d. body: %s", query.Encode(), resp.StatusCode, string(body))

	var out []imageDTO
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"list images?%s: response must decode as a JSON array. body: %s", query.Encode(), string(body))
	return out
}
