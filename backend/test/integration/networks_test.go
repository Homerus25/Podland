//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Networks_List is the networks list vertical slice
// (247). It proves GET /api/hosts/{id}/networks against a real
// podman over the SSH connection seam — not just the fake engine:
//
//	GET /api/hosts/{id}/networks → 200, JSON array
//
// The host is registered through the public API by TestMain (the
// harness boots the test host per the run-scoped-tag isolation model).
// Assertions pin the DTO contract (name, driver, subnets as CIDR
// strings, createdAt) on two fixtures: the default bridge network
// podman pre-creates, and a run-scoped network created via the podman
// CLI over the host connection (scaffolding, not the SUT) so the
// list is proven to reflect the host's real state. Isolation
// follows the house model: run-scoped name, per-test t.Cleanup,
// suite sweep for aborts.
func Test_Integration_Networks_List(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerNetworksSweep()

	hostID := suite.HostID

	// --- AC: the default bridge network podman pre-creates appears
	// with its driver and subnet ----------------------------------------
	t.Run("DefaultBridgeNetworkListed", func(t *testing.T) {
		networks := listNetworks(t, hostID)

		var found *networkDTO
		for i := range networks {
			if networks[i].Name == "podman" {
				found = &networks[i]
				break
			}
		}
		require.NotNil(t, found, "the default bridge network %q must appear in the list, got %d networks", "podman", len(networks))
		assert.Equal(t, "bridge", found.Driver, "the default network must use the bridge driver")
		require.NotEmpty(t, found.Subnets, "the default bridge network must report at least one subnet")
		for _, cidr := range found.Subnets {
			_, _, err := net.ParseCIDR(cidr)
			assert.NoError(t, err, "subnet %q must be a CIDR string", cidr)
		}
		assert.NotEmpty(t, found.CreatedAt, "createdAt must be reported")
	})

	// --- AC: a run-scoped network created over the host connection
	// appears with its configured subnet — the list reflects real podman
	// state, not a canned default ----------------------------------------
	t.Run("RunScopedNetworkAppearsInList", func(t *testing.T) {
		networkName := "it-" + suite.RunID + "-net"
		subnet := "10.99.0.0/24"

		t.Cleanup(func() {
			if out, err := podmanConnRun("network", "rm", networkName); err != nil {
				t.Logf("cleanup: network rm %q best-effort failed: %v (%s)", networkName, err, strings.TrimSpace(out))
			}
		})

		// Fixture: one named run-scoped network with a pinned subnet,
		// created over the same podman connection the backend tunnels
		// through.
		out, err := podmanConnRun("network", "create",
			"--subnet", subnet,
			networkName)
		require.NoError(t, err, "network create must succeed: %s", strings.TrimSpace(out))

		networks := listNetworks(t, hostID)

		var found *networkDTO
		for i := range networks {
			if networks[i].Name == networkName {
				found = &networks[i]
				break
			}
		}
		require.NotNil(t, found, "run-scoped network %q must appear in the list, got %d networks", networkName, len(networks))
		assert.Equal(t, "bridge", found.Driver)
		assert.Contains(t, found.Subnets, subnet, "the configured subnet must be reported as a CIDR string")
		assert.NotEmpty(t, found.CreatedAt, "createdAt must be reported")
	})
}

// --- Access-control behavior (middleware, shared with all downstream
// families): networks rides the same RequireHostAccess subtree, so
// a no-access user gets 403 ACCESS_DENIED and an unknown host ID
// gets 404 HOST_NOT_FOUND, both with the standard error envelope.
func Test_Integration_Networks_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	client := noAccessUserClient(t)

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet,
			suite.ServerURL+"/api/hosts/"+suite.HostID+"/networks",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UnknownHostGets404", func(t *testing.T) {
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodGet,
			suite.ServerURL+"/api/hosts/00000000-0000-0000-0000-000000000000/networks",
			"", http.StatusNotFound, "HOST_NOT_FOUND")
	})
}

// networkDTO mirrors the networks list response item.
type networkDTO struct {
	Name      string   `json:"name"`
	Driver    string   `json:"driver"`
	Subnets   []string `json:"subnets"`
	CreatedAt string   `json:"createdAt"`
}

// listNetworks GETs /api/hosts/{id}/networks and decodes the JSON
// array.
func listNetworks(t *testing.T, hostID string) []networkDTO {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/hosts/" + hostID + "/networks")
	require.NoError(t, err, "GET /networks must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read networks response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list networks: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out []networkDTO
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"networks response must decode as a JSON array. body: %s", string(body))
	return out
}

// --- Suite sweep (covers aborts) ---------------------------------------

var registerNetworksSweepOnce sync.Once

func registerNetworksSweep() {
	registerNetworksSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped networks", func() error {
			out, err := podmanConnRun("network", "ls", "--format", "{{.Name}}")
			if err != nil {
				return nil
			}
			for _, name := range strings.Split(out, "\n") {
				if strings.Contains(name, fmt.Sprintf("it-%s", runID)) {
					_, _ = podmanConnRun("network", "rm", strings.TrimSpace(name))
				}
			}
			return nil
		})
	})
}
