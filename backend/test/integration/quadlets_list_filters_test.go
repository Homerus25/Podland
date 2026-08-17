//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_ListFilters is the quadlet list-filter vertical
// slice for the real-podman integration suite. It proves the query-parameter
// filtering of GET /api/machines/{id}/quadlets end-to-end through handler →
// service → podman QuadletList (contract at doku/description_quadlets_api.md:15-17):
//
//	GET /api/machines/{id}/quadlets?resource=<type>  → only that resource type
//	GET /api/machines/{id}/quadlets?name=<substr>    → case-insensitive substring
//	GET /api/machines/{id}/quadlets?resource&name    → both filters AND-ed
//
// Fixture: two run-scoped quadlets with distinct resources AND distinct name
// substrings (`-lf-web` container, `-lf-data` volume), so every filter
// dimension has both a positive and a negative example on the machine.
// Nothing here starts a unit, so the busybox prerequisite of the other
// quadlet slices is not needed — install only writes the file and reloads
// systemd.
//
// Isolation model matches the CRUD slice (080-08): run-scoped names +
// per-test t.Cleanup DELETE + the shared suite sweep for aborts.
func Test_Integration_Quadlets_ListFilters(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	containerName := "it-" + suite.RunID + "-lf-web"
	volumeName := "it-" + suite.RunID + "-lf-data"

	// Registered before create so it runs even if a later phase fails. The
	// deleteQuadlet helper accepts 404 (idempotent) for the abort case.
	t.Cleanup(func() {
		deleteQuadlet(t, machineID, containerName+".container")
		deleteQuadlet(t, machineID, volumeName+".volume")
	})

	createQuadlet(t, machineID, quadletCreateBody{
		Name:     containerName,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	})
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     volumeName,
		Resource: "volume",
		Content:  "[Volume]\nDriver=local\n",
	})

	// Prove the fixtures are visible unfiltered first, so the filter
	// assertions below can't pass vacuously through a broken list endpoint.
	t.Run("UnfilteredListShowsBothFixtures", func(t *testing.T) {
		entries := listQuadletsFiltered(t, machineID, nil)
		names := quadletEntryNames(entries)
		require.Contains(t, names, containerName+".container",
			"unfiltered list must include the container fixture")
		require.Contains(t, names, volumeName+".volume",
			"unfiltered list must include the volume fixture")
	})

	// --- AC #1: resource filter narrows by resource type ---------------
	t.Run("ResourceFilterNarrowsByResourceType", func(t *testing.T) {
		entries := listQuadletsFiltered(t, machineID, url.Values{"resource": {"container"}})
		names := quadletEntryNames(entries)
		require.Contains(t, names, containerName+".container",
			"resource=container must include the container fixture")
		require.NotContains(t, names, volumeName+".volume",
			"resource=container must exclude the volume fixture")
		for _, entry := range entries {
			require.Equal(t, "container", entry.Resource,
				"resource=container must only return container quadlets")
		}

		entries = listQuadletsFiltered(t, machineID, url.Values{"resource": {"volume"}})
		names = quadletEntryNames(entries)
		require.Contains(t, names, volumeName+".volume",
			"resource=volume must include the volume fixture")
		require.NotContains(t, names, containerName+".container",
			"resource=volume must exclude the container fixture")
		for _, entry := range entries {
			require.Equal(t, "volume", entry.Resource,
				"resource=volume must only return volume quadlets")
		}
	})

	// --- AC #2: name filter is a case-insensitive substring match ------
	t.Run("NameFilterMatchesSubstringCaseInsensitively", func(t *testing.T) {
		entries := listQuadletsFiltered(t, machineID, url.Values{"name": {"lf-web"}})
		names := quadletEntryNames(entries)
		require.Contains(t, names, containerName+".container",
			"name=lf-web must include the container fixture")
		require.NotContains(t, names, volumeName+".volume",
			"name=lf-web must exclude the volume fixture")

		entries = listQuadletsFiltered(t, machineID, url.Values{"name": {"LF-DATA"}})
		names = quadletEntryNames(entries)
		require.Contains(t, names, volumeName+".volume",
			"name=LF-DATA must still match the volume fixture (case-insensitive)")
		require.NotContains(t, names, containerName+".container",
			"name=LF-DATA must exclude the container fixture")
	})

	// --- AC #3: combined filters are AND-ed ----------------------------
	t.Run("CombinedFiltersAreAnded", func(t *testing.T) {
		// `lf-data` matches the volume fixture by name, but resource=container
		// excludes it: the intersection of both filters is empty.
		entries := listQuadletsFiltered(t, machineID, url.Values{
			"resource": {"container"},
			"name":     {"lf-data"},
		})
		require.Empty(t, entries,
			"resource=container AND name=lf-data must match nothing (the only lf-data quadlet is a volume)")

		// Both filters agreeing must still return the quadlet.
		entries = listQuadletsFiltered(t, machineID, url.Values{
			"resource": {"container"},
			"name":     {"lf-web"},
		})
		names := quadletEntryNames(entries)
		require.Contains(t, names, containerName+".container",
			"resource=container AND name=lf-web must include the container fixture")
	})

	// --- AC #4: a filter with no matches returns an empty list ---------
	t.Run("NoMatchReturnsEmptyList", func(t *testing.T) {
		entries := listQuadletsFiltered(t, machineID, url.Values{
			"name": {"it-" + suite.RunID + "-lf-nosuchquadlet"},
		})
		require.Empty(t, entries,
			"a name filter matching nothing must return an empty list, not an error")
	})
}

// listQuadletsFiltered GETs /quadlets with the given query parameters and
// returns the decoded entries. A nil query lists unfiltered. The response is
// a bare JSON array ([]QuadletListResponse) matching the other quadlet
// helpers; an all-filtered-out list may decode as nil.
func listQuadletsFiltered(t *testing.T, machineID string, query url.Values) []quadletListResponse {
	t.Helper()

	listURL := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/"
	if len(query) > 0 {
		listURL += "?" + query.Encode()
	}
	resp, err := suite.AuthenticatedClient().Get(listURL)
	require.NoErrorf(t, err, "GET /quadlets?%s must connect to the in-process server", query.Encode())
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list quadlets?%s: expected 200, got %d. body: %s", query.Encode(), resp.StatusCode, string(body))

	var result []quadletListResponse
	require.NoErrorf(t, json.Unmarshal(body, &result),
		"list quadlets?%s: response must decode as a JSON array. body: %s", query.Encode(), string(body))
	return result
}

// quadletEntryNames collects the names of the list entries for the
// presence/absence assertions (podman reports quadlet names with extension).
func quadletEntryNames(entries []quadletListResponse) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}
