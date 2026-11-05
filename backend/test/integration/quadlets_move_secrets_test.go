//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet move/copy secret slice (issue 215). Every `Secret=` line of
// the quadlet content counts: the preview always reports the references
// with their existence in each host's podman secret store, and the
// transfer carries VALUES only when the request opts into
// transferSecrets — admin on the source, a same-name secret on the target
// is skipped not overwritten, dangling and host-path references are
// skipped and reported, and a move never removes the source secrets.
//
// Everything here is API-driven on both hosts: secrets are created
// (POST /secrets), verified (list) and compared (admin reveal) through the
// in-process server, so the slice runs wherever the two-host harness
// runs. The user-systemd probe gates the quadlet surface, as in every
// quadlet slice.

// revealSecretViaAPI GETs ?reveal=true as the owner and returns the
// value — the byte-identical comparison's source of truth.
func revealSecretViaAPI(t *testing.T, hostID, secretName string) string {
	t.Helper()
	revealURL := suite.ServerURL + "/api/hosts/" + hostID + "/secrets/" + url.PathEscape(secretName) + "?reveal=true"
	resp, err := suite.AuthenticatedClient().Get(revealURL)
	require.NoError(t, err, "admin reveal must connect to the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"reveal %s on %s: expected 200, got %d. body: %s", secretName, hostID, resp.StatusCode, string(body))
	var got secretInspectResponse
	require.NoErrorf(t, json.Unmarshal(body, &got), "reveal: response must decode. body: %s", string(body))
	require.NotNil(t, got.SecretData, "an admin reveal must include the secretData field")
	return *got.SecretData
}

// postMoveAs posts a transfer body as the given client (the below-admin
// slice drives the endpoint with a write-level grantee) and returns the
// response's status and body.
func postMoveAs(t *testing.T, client *http.Client, sourceHostID, quadletName, body string) (int, []byte) {
	t.Helper()
	resp, err := client.Post(moveURL(sourceHostID, quadletName),
		"application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err, "POST /quadlets/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// belowAdminOnBothClient seeds a user holding WRITE on both hosts —
// enough for a plain transfer on every gate, one level short of moving a
// secret value off the source.
func belowAdminOnBothClient(t *testing.T, hostB string) *http.Client {
	t.Helper()
	username := "it-" + suite.RunID + "-mvsec-w"
	password := "it-" + suite.RunID + "-mvsec-w-pw-" + uuid.New().String()[:8]
	userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
	require.NoErrorf(t, err, "seed write user: %v", err)
	token, err := realLogin(suite.ServerURL, username, password)
	require.NoErrorf(t, err, "login as write user: %v", err)
	for _, hostID := range []string{suite.HostID, hostB} {
		grantURL := suite.ServerURL + "/api/hosts/" + hostID + "/access"
		grantBody := fmt.Sprintf(`{"userId":%q,"permission":%q}`, userID, models.PermissionWrite)
		resp, err := suite.AuthenticatedClient().Post(grantURL, "application/json", bytes.NewReader([]byte(grantBody)))
		require.NoErrorf(t, err, "grant write on %s: %v", hostID, err)
		_ = resp.Body.Close()
		require.Containsf(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode,
			"grant write on %s: unexpected status %d", hostID, resp.StatusCode)
	}
	return bearerClient(token)
}

// bearerClient wraps a token in a client, the same shape the level-user
// fixtures build.
func bearerClient(token string) *http.Client {
	return &http.Client{
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
	}
}

// secretTransferEnvelope mirrors the fields of the transfer envelope this
// slice asserts.
type secretTransferEnvelope struct {
	Status         string `json:"status"`
	SecretTransfer *struct {
		Secrets []struct {
			Name    string `json:"name"`
			Outcome string `json:"outcome"`
			Reason  string `json:"reason"`
		} `json:"secrets"`
	} `json:"secretTransfer"`
}

func Test_Integration_Quadlets_MoveSecrets(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()
	registerSecretsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondHost(t)
	if !b.Independent {
		t.Skip("host B shares host A's host (PODLAND_TEST_SSH_* fallback) — secret transfers need independent podman state")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("host B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	hostA := suite.HostID
	hostB := b.HostID
	base := "it-" + suite.RunID + "-mvsec"

	// secretLine builds one quadlet Secret= line for a run-scoped name.
	secretLine := func(suffix string) string { return base + "-" + suffix }

	// cleanupSecret best-effort removes the secret from both hosts.
	cleanupSecret := func(name string) {
		t.Cleanup(func() { deleteSecretBestEffort(t, hostA, name) })
		t.Cleanup(func() { deleteSecretBestEffort(t, hostB, name) })
	}

	// --- AC: the preview reports per-store existence; hostPath and
	// dangling references are honest entries ----------------------------
	t.Run("PreviewReportsSecretExistence", func(t *testing.T) {
		quadlet := base + "-p"
		transferable := secretLine("tok")
		dangling := secretLine("gone")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(transferable)
		createSecret(t, hostA, secretCreateBody{Name: transferable, Value: "preview-value"})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + transferable + `
Secret=/etc/certs/store.pem
Secret=` + dangling + `
`,
		})

		got := getMovePreview(t, hostA, quadlet+".container", hostB)
		require.Len(t, got.Secrets, 3)
		assert.Equal(t, transferable, got.Secrets[0].Name)
		assert.Equal(t, "secret", got.Secrets[0].Kind)
		assert.True(t, got.Secrets[0].ExistsOnSource)
		assert.False(t, got.Secrets[0].ExistsOnTarget, "host B holds no such secret yet")
		assert.Equal(t, "/etc/certs/store.pem", got.Secrets[1].Name)
		assert.Equal(t, "hostPath", got.Secrets[1].Kind)
		assert.False(t, got.Secrets[1].ExistsOnSource, "a hostPath entry carries no existence claims")
		assert.False(t, got.Secrets[1].ExistsOnTarget)
		assert.Equal(t, dangling, got.Secrets[2].Name)
		assert.False(t, got.Secrets[2].ExistsOnSource, "a dangling reference is an honest entry, not an error")
		assert.False(t, got.Secrets[2].ExistsOnTarget)

		// Host B gains the same-name secret: both flags flip true.
		createSecret(t, hostB, secretCreateBody{Name: transferable, Value: "on-b"})
		got = getMovePreview(t, hostA, quadlet+".container", hostB)
		assert.True(t, got.Secrets[0].ExistsOnSource)
		assert.True(t, got.Secrets[0].ExistsOnTarget)
	})

	// --- AC: copy with transferSecrets creates the secret on the target,
	// value byte-identical, source unchanged ----------------------------
	t.Run("CopyTransfersTheValue", func(t *testing.T) {
		quadlet := base + "-c"
		secretName := secretLine("carry")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(secretName)
		createSecret(t, hostA, secretCreateBody{Name: secretName, Value: "carry-me-value\n"})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + secretName + `
`,
		})

		status, raw := postMove(t, hostA, quadlet+".container",
			moveCompanionBody(t, hostB, "copy", map[string]any{"transferSecrets": true}))
		require.Equalf(t, http.StatusOK, status, "the copy must succeed. body: %s", string(raw))
		var got secretTransferEnvelope
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "copied", got.Status)
		require.NotNil(t, got.SecretTransfer, "the envelope carries the secretTransfer block")
		require.Len(t, got.SecretTransfer.Secrets, 1)
		assert.Equal(t, secretName, got.SecretTransfer.Secrets[0].Name)
		assert.Equal(t, "transferred", got.SecretTransfer.Secrets[0].Outcome)

		// The value crossed hosts byte-identical (newline included).
		assert.Equal(t, "carry-me-value\n", revealSecretViaAPI(t, hostB, secretName))
		// The source secret is untouched.
		requireSecretListed(t, hostA, secretName, "the source secret survives a copy")
		// And the quadlet landed on B.
		getQuadlet(t, hostB, quadlet+".container")
	})

	// --- AC: a same-name secret on the target is skipped, never
	// overwritten --------------------------------------------------------
	t.Run("SameNameOnTargetIsSkipped", func(t *testing.T) {
		quadlet := base + "-s"
		secretName := secretLine("clash")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(secretName)
		createSecret(t, hostA, secretCreateBody{Name: secretName, Value: "from-a"})
		createSecret(t, hostB, secretCreateBody{Name: secretName, Value: "from-b"})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + secretName + `
`,
		})

		status, raw := postMove(t, hostA, quadlet+".container",
			moveCompanionBody(t, hostB, "copy", map[string]any{"transferSecrets": true}))
		require.Equalf(t, http.StatusOK, status, "a same-name skip must not fail the transfer. body: %s", string(raw))
		var got secretTransferEnvelope
		require.NoError(t, json.Unmarshal(raw, &got))
		require.Len(t, got.SecretTransfer.Secrets, 1)
		assert.Equal(t, "skipped", got.SecretTransfer.Secrets[0].Outcome)
		assert.NotEmpty(t, got.SecretTransfer.Secrets[0].Reason)

		// Host B's secret keeps its own value.
		assert.Equal(t, "from-b", revealSecretViaAPI(t, hostB, secretName))
	})

	// --- AC: a dangling reference skips and reports, not errors ---------
	t.Run("DanglingReferenceSkips", func(t *testing.T) {
		quadlet := base + "-d"
		dangling := secretLine("missing")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + dangling + `
`,
		})

		status, raw := postMove(t, hostA, quadlet+".container",
			moveCompanionBody(t, hostB, "copy", map[string]any{"transferSecrets": true}))
		require.Equalf(t, http.StatusOK, status, "a dangling reference must not fail the transfer. body: %s", string(raw))
		var got secretTransferEnvelope
		require.NoError(t, json.Unmarshal(raw, &got))
		require.Len(t, got.SecretTransfer.Secrets, 1)
		assert.Equal(t, dangling, got.SecretTransfer.Secrets[0].Name)
		assert.Equal(t, "skipped", got.SecretTransfer.Secrets[0].Outcome)
		assert.Contains(t, got.SecretTransfer.Secrets[0].Reason, "does not exist")

		getQuadlet(t, hostB, quadlet+".container")
	})

	// --- AC: transferSecrets below admin on the source is 403 in the
	// preflight, nothing moves -------------------------------------------
	t.Run("BelowAdminSourceIs403", func(t *testing.T) {
		quadlet := base + "-w"
		secretName := secretLine("guarded")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(secretName)
		createSecret(t, hostA, secretCreateBody{Name: secretName, Value: "must-not-move"})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + secretName + `
`,
		})
		client := belowAdminOnBothClient(t, hostB)

		status, raw := postMoveAs(t, client, hostA, quadlet+".container",
			moveCompanionBody(t, hostB, "copy", map[string]any{"transferSecrets": true}))
		require.Equalf(t, http.StatusForbidden, status,
			"a write-level caller must be refused. body: %s", string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)

		// Nothing moved in either direction.
		getQuadletExpectNotFound(t, hostB, quadlet+".container")
		quadletListsEntry(t, hostA, quadlet+".container")
		requireSecretNotListed(t, hostB, secretName, "no value moved off the source")
	})

	// --- AC: a move removes only the quadlet; source secrets survive -----
	t.Run("MoveKeepsSourceSecrets", func(t *testing.T) {
		quadlet := base + "-mv"
		secretName := secretLine("stay")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(secretName)
		createSecret(t, hostA, secretCreateBody{Name: secretName, Value: "i-stay"})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Secret=` + secretName + `
`,
		})

		status, raw := postMove(t, hostA, quadlet+".container",
			moveCompanionBody(t, hostB, "move", map[string]any{"transferSecrets": true}))
		require.Equalf(t, http.StatusOK, status, "the move must succeed. body: %s", string(raw))
		var got secretTransferEnvelope
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, "moved", got.Status)

		getQuadletExpectNotFound(t, hostA, quadlet+".container")
		getQuadlet(t, hostB, quadlet+".container")
		// The secret never leaves the source, any mode.
		requireSecretListed(t, hostA, secretName, "a move removes ONLY the quadlet — source secrets survive")
		assert.Equal(t, "i-stay", revealSecretViaAPI(t, hostA, secretName))
	})

	// --- AC: with migrateVolumeData also set, the NDJSON terminal
	// complete event carries both blocks ----------------------------------
	t.Run("BlocksRideTheNDJSONTerminalEvent", func(t *testing.T) {
		quadlet := base + "-nd"
		volume := base + "-vol"
		secretName := secretLine("stream")

		for _, host := range []string{hostA, hostB} {
			name := quadlet + ".container"
			t.Cleanup(func() { forceDeleteQuadlet(t, host, name) })
		}
		cleanupSecret(secretName)
		createSecret(t, hostA, secretCreateBody{Name: secretName, Value: "rides-along"})
		createVolumeViaAPI(t, hostA, volume)
		seedVolumeViaAPI(t, hostA, volume, map[string][]byte{"note.txt": []byte("with secrets")})
		createQuadlet(t, hostA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content: `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Volume=` + volume + `:/data:Z
Secret=` + secretName + `
`,
		})

		events := postMoveMigrate(t, hostA, quadlet+".container",
			migratingMoveBody(t, hostB, "copy", map[string]any{"transferSecrets": true}))

		complete := events[len(events)-1]
		assert.Equal(t, "complete", complete.Type)
		require.NotNil(t, complete.Result)
		assert.Equal(t, "copied", complete.Result.Status)
		require.NotNil(t, complete.Result.VolumeMigration, "the volumeMigration block rides the terminal event")
		require.Len(t, complete.Result.VolumeMigration.Volumes, 1)
		assert.Equal(t, "migrated", complete.Result.VolumeMigration.Volumes[0].Outcome)
		require.NotNil(t, complete.Result.SecretTransfer, "the secretTransfer block rides beside it")
		require.Len(t, complete.Result.SecretTransfer.Secrets, 1)
		assert.Equal(t, secretName, complete.Result.SecretTransfer.Secrets[0].Name)
		assert.Equal(t, "transferred", complete.Result.SecretTransfer.Secrets[0].Outcome)

		assert.Equal(t, "rides-along", revealSecretViaAPI(t, hostB, secretName))
	})
}
