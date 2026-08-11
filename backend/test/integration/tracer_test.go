//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_Integration_TracerBullet is the canonical tracer bullet for the
// Podland integration suite. It issues a single HTTP call against the
// real in-process backend:
//
//	GET /api/machines/{id}/containers
//
// on the once-registered, ready machine and asserts 200 + an empty
// container list. That single assertion exercises the entire pipeline:
//
//	HTTP → middleware JWT validation → machine access check
//	     → connection-manager acquire → connection-factory decrypt SSH key
//	     → SSH tunnel to the podman machine → podman socket
//	     → ContainerList → JSON encode → HTTP response
//
// If anything in that chain is broken (auth misconfigured, SSH key
// perms wrong, socket cold, machine unreachable, …) this test fails and
// surfaces the real error rather than a downstream symptom.
//
// Per the PRD's "surface, don't silently work around" rule, an SSH
// failure here is reported as-is: the known connection_factory.go gap
// (decrypted key written without chmod 0600) bites here if it bites
// anywhere, and we want to see it.
func Test_Integration_TracerBullet(t *testing.T) {
	suite.SkipIfUnavailable(t)

	url := suite.ServerURL + "/api/machines/" + suite.MachineID + "/containers/"

	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET containers must connect to the in-process server")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read containers response body")

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"tracer bullet: expected 200 from GET containers, got %d. body: %s",
		resp.StatusCode, string(body))

	// The empty-list assertion is what makes this a tracer bullet: any
	// 5xx or unexpected payload would point at a specific layer. The
	// response shape is containers.ContainerListResult — an object with
	// a `containers` field — so decode through that envelope.
	var result struct {
		Containers []any `json:"containers"`
		Total      int   `json:"total"`
	}
	require.NoError(t, json.Unmarshal(body, &result),
		"tracer bullet: response must decode as a ContainerListResult. body: %s", string(body))
	assert.Empty(t, result.Containers,
		"tracer bullet: expected empty container list on a clean machine. body: %s", string(body))
	assert.Equal(t, 0, result.Total,
		"tracer bullet: expected total=0 on a clean machine. body: %s", string(body))
}
