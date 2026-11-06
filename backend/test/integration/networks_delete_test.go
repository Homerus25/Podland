//go:build integration

package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Networks_Delete is the network delete vertical slice
// (issue 460) against DELETE /api/hosts/{id}/networks/{networkName}?force=:
//
//	DELETE unused network             → 204, gone from the list
//	DELETE in-use network             → 409 NETWORK_IN_USE
//	DELETE in-use network ?force=true → 204, container removed, network gone
//	DELETE unknown network            → 404 NETWORK_NOT_FOUND
func Test_Integration_Networks_Delete(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerNetworksSweep()
	registerContainersSweep()

	hostID := suite.HostID

	// --- AC: unused network deletes with 204 ---------------------------
	t.Run("UnusedNetworkDeletes204", func(t *testing.T) {
		networkName := "it-" + suite.RunID + "net-del-unused"
		out, err := podmanConnRun("network", "create", networkName)
		require.NoError(t, err, "fixture network create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() {
			_, _ = podmanConnRun("network", "rm", "-f", networkName)
		})

		deleteNetwork(t, hostID, networkName, "", http.StatusNoContent)

		assert.NotContains(t, listNetworkNames(t, hostID), networkName,
			"deleted network must disappear from the list")
	})

	// --- AC: in-use network without force gets 409; with force succeeds -
	t.Run("InUseNetwork", func(t *testing.T) {
		networkName := "it-" + suite.RunID + "net-del-inuse"
		containerName := "it-" + suite.RunID + "net-del-inuse-c"
		out, err := podmanConnRun("network", "create", networkName)
		require.NoError(t, err, "fixture network create must succeed: %s", strings.TrimSpace(out))
		t.Cleanup(func() {
			_, _ = podmanConnRun("rm", "-f", containerName)
			_, _ = podmanConnRun("network", "rm", "-f", networkName)
		})

		// Fixture: a running container attached to the network. busybox
		// is ensured by the shared helper (same prerequisite as
		// containers).
		ensureBusyboxImage(t)
		out, err = podmanConnRun("run", "-d", "--name", containerName,
			"--network", networkName, "busybox:latest", "sh", "-c", "sleep 600")
		require.NoError(t, err, "fixture container create must succeed: %s", strings.TrimSpace(out))
		require.NotEmpty(t, strings.TrimSpace(out), "fixture container must print an id")

		// Without force: podman refuses → 409 with the distinct code.
		body := deleteNetwork(t, hostID, networkName, "", http.StatusConflict)
		assert.Contains(t, body, "NETWORK_IN_USE", "in-use refusal must carry the distinct error code")

		// With force: podman removes the attached container — stopping it
		// first, since it is running — and then the network. The container
		// assertion pins the real blast radius: force does not merely
		// disconnect, it destroys.
		deleteNetwork(t, hostID, networkName, "?force=true", http.StatusNoContent)

		assert.NotContains(t, listNetworkNames(t, hostID), networkName,
			"force-deleted network must be gone from the list")
		_, err = podmanConnRun("container", "exists", containerName)
		assert.Error(t, err,
			"force delete must remove the attached container, not just disconnect it")
	})

	// --- AC: unknown network name returns the clean 404 ----------------
	t.Run("UnknownNameGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+hostID+"/networks/it-"+suite.RunID+"net-no-such-network",
			"", http.StatusNotFound, "NETWORK_NOT_FOUND")
	})
}

// Test_Integration_Networks_Delete_AccessControl pins the write gate on
// the delete route: read-level grantees and no-access users get 403,
// unknown host gets 404.
func Test_Integration_Networks_Delete_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("ReadLevelUserGets403", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/networks/it-"+suite.RunID+"net-whatever",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodDelete,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/networks/it-"+suite.RunID+"net-whatever",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownHostGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodDelete,
			suite.ServerURL+"/api/hosts/00000000-0000-0000-0000-000000000000/networks/whatever",
			"", http.StatusNotFound, "HOST_NOT_FOUND")
	})
}

// deleteNetwork DELETEs a network with an optional raw query string and
// asserts the expected status; it returns the response body text.
func deleteNetwork(t *testing.T, hostID, networkName, query string, wantStatus int) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete,
		suite.ServerURL+"/api/hosts/"+hostID+"/networks/"+networkName+query, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE network must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read network delete response body")
	require.Equalf(t, wantStatus, resp.StatusCode,
		"delete network %q: expected %d, got %d. body: %s", networkName, wantStatus, resp.StatusCode, string(body))
	return string(body)
}
