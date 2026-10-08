//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Networks_CreateConnect is the networks mutation
// vertical slice (257). It proves the two write endpoints against a
// real podman over the SSH connection seam — not just the fake engine:
//
//	POST /api/machines/{id}/networks                    → 201 network DTO
//	POST /api/machines/{id}/networks/{name}/connect     → 200 empty body
//	POST /api/machines/{id}/networks (duplicate name)    → 409 NETWORK_ALREADY_EXISTS
//
// The machine is registered through the public API by TestMain (the
// harness boots the test host per the run-scoped-tag isolation model).
// The flow creates a run-scoped network through the API, starts a
// run-scoped container (the established pull-and-sleep pattern), connects
// it, and asserts the attachment from the containers list — the list DTO
// that carries each container's networks (the networks list DTO carries
// no attachment info). The duplicate-name 409 is covered against the
// real podman.
//
// Isolation follows the house model: run-scoped names, per-test
// t.Cleanup, suite sweeps for aborts.
func Test_Integration_Networks_CreateConnect(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerNetworksSweep()
	registerContainersSweep()

	machineID := suite.MachineID

	// Run-scoped names so list assertions and the suite sweeps can find
	// exactly what this run created.
	networkName := "it-" + suite.RunID + "-connect"
	containerName := "it-" + suite.RunID + "-connect-c"

	// --- AC: create a run-scoped network through the API; 201 with
	// the list DTO contract (driver, subnets as CIDR, createdAt) ------
	t.Run("CreateReturns201WithNetworkDTO", func(t *testing.T) {
		created := createNetwork(t, machineID, networkName)

		assert.Equal(t, networkName, created.Name, "create must answer the requested network name")
		assert.Equal(t, "bridge", created.Driver, "podman's default driver must be bridge")
		require.NotEmpty(t, created.Subnets, "the created network must report at least one subnet")
		for _, cidr := range created.Subnets {
			_, _, err := net.ParseCIDR(cidr)
			assert.NoError(t, err, "subnet %q must be a CIDR string", cidr)
		}
		assert.NotEmpty(t, created.CreatedAt, "createdAt must be reported")
	})
	// Fail fast: every later phase depends on the created network.
	if t.Failed() {
		return
	}

	// --- AC: the created network appears in the networks list --------
	t.Run("CreatedNetworkAppearsInList", func(t *testing.T) {
		networks := listNetworks(t, machineID)

		var found *networkDTO
		for i := range networks {
			if networks[i].Name == networkName {
				found = &networks[i]
				break
			}
		}
		require.NotNil(t, found, "created network %q must appear in the list, got %d networks", networkName, len(networks))
		assert.Equal(t, "bridge", found.Driver)
		assert.NotEmpty(t, found.CreatedAt, "createdAt must be reported")
	})
	if t.Failed() {
		return
	}

	// --- AC: start a run-scoped container (pull-and-sleep) ----------
	// Prerequisite: busybox must exist on the machine before
	// ContainerCreate will succeed (podman does not auto-pull on
	// create). Idempotent; see the containers slice.
	ensureBusyboxImage(t)

	// Per-test cleanup, registered BEFORE create so it runs even if a
	// later phase fails. Best-effort, in dependency order: the container
	// must be force-removed before the network (podman refuses to remove
	// a network that still has containers attached). The suite sweeps
	// are the backstop for the abort case.
	var containerID string
	t.Cleanup(func() {
		if containerID != "" {
			if out, err := podmanConnRun("rm", "-f", containerID); err != nil {
				t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", containerID, err, strings.TrimSpace(out))
			}
		}
		if out, err := podmanConnRun("network", "rm", networkName); err != nil {
			t.Logf("cleanup: network rm %q best-effort failed: %v (%s)", networkName, err, strings.TrimSpace(out))
		}
	})

	t.Run("CreateAndStartContainer", func(t *testing.T) {
		containerID = createContainer(t, machineID, containerCreateBody{
			Name:    containerName,
			Image:   "busybox:latest",
			Command: []string{"sh", "-c", "sleep 600"},
		})
		startContainer(t, machineID, containerID)
		waitForContainerState(t, machineID, containerID, "running",
			"container must reach running state after start")
	})
	// Fail fast: the connect phase depends on a running container.
	if t.Failed() {
		return
	}

	// --- AC: connect the container; 200 with an empty body ----------
	t.Run("ConnectReturns200EmptyBody", func(t *testing.T) {
		connectContainerToNetwork(t, machineID, networkName, containerID)
	})

	// --- AC: the attachment is reflected in the containers list ------
	// The networks list DTO carries no attachment info; the containers
	// list DTO carries each container's networks — that is where the
	// attachment is asserted.
	t.Run("AttachmentReflectedInContainerList", func(t *testing.T) {
		requireContainerNetworksContain(t, machineID, containerID, networkName)
	})

	// --- AC: a duplicate create answers 409 NETWORK_ALREADY_EXISTS --
	t.Run("DuplicateCreateReturns409", func(t *testing.T) {
		payload, err := json.Marshal(networkCreateBody{Name: networkName})
		require.NoError(t, err)
		expectAPIError(t, suite.AuthenticatedClient(), http.MethodPost,
			suite.ServerURL+"/api/machines/"+machineID+"/networks",
			string(payload), http.StatusConflict, "NETWORK_ALREADY_EXISTS")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers — each hits the in-process backend via the authenticated client
// ---------------------------------------------------------------------------

// networkCreateBody mirrors the JSON-accepted fields of
// networks.NetworkCreateRequest that this slice populates. Kept as a
// local type so the integration package doesn't import the networks
// domain package (matching the containers slice style).
type networkCreateBody struct {
	Name       string   `json:"name"`
	Containers []string `json:"containers,omitempty"`
}

// networkConnectBody mirrors networks.NetworkConnectRequest.
type networkConnectBody struct {
	Container string `json:"container"`
}

// createNetwork POSTs /networks and returns the created network DTO.
// Asserts 201 (the handler writes StatusCreated on success).
func createNetwork(t *testing.T, machineID, name string) networkDTO {
	t.Helper()
	payload, err := json.Marshal(networkCreateBody{Name: name})
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/networks"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /networks must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"create network: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var created networkDTO
	require.NoErrorf(t, json.Unmarshal(raw, &created),
		"create network: response must decode as the network list DTO. body: %s", string(raw))
	return created
}

// connectContainerToNetwork POSTs /networks/{name}/connect and asserts
// 200 with an empty body (the handler's struct{} result encodes as {},
// the same "empty body" the handler contract test pins).
func connectContainerToNetwork(t *testing.T, machineID, networkName, containerID string) {
	t.Helper()
	payload, err := json.Marshal(networkConnectBody{Container: containerID})
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/networks/" + networkName + "/connect"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /networks/{name}/connect must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"connect container: expected 200, got %d. body: %s", resp.StatusCode, string(raw))
	assert.JSONEqf(t, `{}`, strings.TrimSpace(string(raw)),
		"connect answers an empty body. body: %s", string(raw))
}

// requireContainerNetworksContain asserts that the container's networks
// array in the containers list contains networkName. The containers list
// DTO carries each container's networks — the seam where an attachment
// is observable (the networks list DTO carries no attachment info).
func requireContainerNetworksContain(t *testing.T, machineID, containerID, networkName string) {
	t.Helper()

	url := suite.ServerURL + "/api/machines/" + machineID + "/containers/?all=true"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /containers/?all=true")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list containers: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	// ContainerListResult envelope: { containers: [...], total, timestamp }.
	var result struct {
		Containers []struct {
			ID       string   `json:"id"`
			Networks []string `json:"networks"`
		} `json:"containers"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &result),
		"list containers: response must decode as ContainerListResult. body: %s", string(body))

	for _, c := range result.Containers {
		if c.ID == containerID {
			require.Containsf(t, c.Networks, networkName,
				"container %s must list network %q among its networks %v", containerID, networkName, c.Networks)
			return
		}
	}
	t.Fatalf("container %s not found in the containers list", containerID)
}
