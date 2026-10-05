//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cross-machine secret copy/move slice (issue 239): the shared
// transfer kit's third composition, behind the secrets domain. A copy
// reveals the value on the source (admin) and creates the secret on the
// target; a move does the same and then removes the source secret —
// install-first-then-remove. The collision is re-checked at execution time
// (409, never an overwrite), and the preview reports the dialog's facts:
// the collision flag and the source quadlets referencing the secret (a
// warning, never a block).
//
// The partial envelope (target install kept, source removal failed) is
// pinned at the service seam — podman v6 has no secret-in-use refusal on
// rm, so a removal failure cannot be simulated through the real podman
// surface without breaking the machine (the quadlet move-mode slice's
// precedent).
//
// Everything here is API-driven on both machines: secrets are created
// (POST /secrets), verified (list) and compared (admin reveal) through the
// in-process server, so the slice runs wherever the two-machine harness
// runs. Only the referencing-quadlet preview fact needs the quadlet
// surface's systemd probe, and that one subtest gates itself.

// secretMoveEnvelope mirrors the plain JSON transfer envelope; only the
// fields this slice asserts are named.
type secretMoveEnvelope struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	SourceMachineID string `json:"sourceMachineId"`
	TargetMachineID string `json:"targetMachineId"`
	Error           string `json:"error,omitempty"`
}

// secretMovePreviewResponse mirrors the preview's JSON shape; only the
// fields this slice asserts are named.
type secretMovePreviewResponse struct {
	Collision           bool `json:"collision"`
	ReferencingQuadlets []struct {
		Name string `json:"name"`
	} `json:"referencingQuadlets"`
}

// secretMoveBody builds a secret transfer request body.
func secretMoveBody(t *testing.T, targetMachineID, mode string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"targetMachineId": targetMachineID, "mode": mode})
	require.NoError(t, err)
	return string(body)
}

// postSecretMove posts a transfer body to the secret move endpoint and
// returns the response's status and body. The secret is addressed by NAME,
// the row's key.
func postSecretMove(t *testing.T, machineID, secretName, body string) (int, []byte) {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Post(
		suite.ServerURL+"/api/machines/"+machineID+"/secrets/"+url.PathEscape(secretName)+"/move",
		"application/json", bytes.NewReader([]byte(body)))
	require.NoError(t, err, "POST /secrets/{id}/move must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// getSecretMovePreview GETs the move preview and returns the decoded
// response.
func getSecretMovePreview(t *testing.T, machineID, secretName, targetMachineID string) secretMovePreviewResponse {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(
		suite.ServerURL + "/api/machines/" + machineID + "/secrets/" + url.PathEscape(secretName) +
			"/move-preview?targetMachineId=" + targetMachineID)
	require.NoError(t, err, "GET move-preview must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"move preview: expected 200, got %d. body: %s", resp.StatusCode, string(raw))
	var got secretMovePreviewResponse
	require.NoErrorf(t, json.Unmarshal(raw, &got),
		"move preview must decode. body: %s", string(raw))
	return got
}

// Test_Integration_Secrets_Move pins the cross-machine secret copy/move
// contract end to end: a copy carries the value byte-identical and leaves
// the source, a move carries the value and removes the source secret, a
// same-named target secret refuses with 409 (target value untouched),
// and the preview reports the dialog's facts.
func Test_Integration_Secrets_Move(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerSecretsSweep()
	registerQuadletsSweep()

	b := suite.RegisterSecondMachine(t)
	if !b.Independent {
		t.Skip("machine B shares machine A's host (PODLAND_TEST_SSH_* fallback) — secret transfers need independent podman state")
	}

	machineA := suite.MachineID
	base := "it-" + suite.RunID + "-secmv"

	// cleanupSecret best-effort removes the secret from both machines —
	// the fixture discipline for secrets this slice creates.
	cleanupSecret := func(name string) {
		t.Cleanup(func() { deleteSecretBestEffort(t, machineA, name) })
		t.Cleanup(func() { deleteSecretBestEffort(t, b.MachineID, name) })
	}

	// --- AC: a copy carries the value, the source stays -----------------
	t.Run("CopyCarriesTheValue", func(t *testing.T) {
		secretName := base + "-copy"
		cleanupSecret(secretName)
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "carry-me-value\n"})

		status, raw := postSecretMove(t, machineA, secretName,
			secretMoveBody(t, b.MachineID, "copy"))
		require.Equalf(t, http.StatusOK, status, "the copy must succeed. body: %s", string(raw))
		var got secretMoveEnvelope
		require.NoError(t, json.Unmarshal(raw, &got), "the envelope must decode. body: %s", string(raw))
		assert.Equal(t, secretName, got.Name)
		assert.Equal(t, "copied", got.Status)
		assert.Equal(t, machineA, got.SourceMachineID)
		assert.Equal(t, b.MachineID, got.TargetMachineID)
		assert.Empty(t, got.Error, "a clean copy reports no error detail")

		// The value crossed machines byte-identical, newline included,
		// and the source secret keeps its copy — a copy never touches
		// the source.
		assert.Equal(t, "carry-me-value\n", revealSecretViaAPI(t, b.MachineID, secretName))
		requireSecretListed(t, machineA, secretName, "the source secret survives a copy")
	})

	// --- AC: a move carries the value, then removes the source secret ----
	t.Run("MoveRemovesSourceSecret", func(t *testing.T) {
		secretName := base + "-mv"
		cleanupSecret(secretName)
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "move-me-value"})

		status, raw := postSecretMove(t, machineA, secretName,
			secretMoveBody(t, b.MachineID, "move"))
		require.Equalf(t, http.StatusOK, status, "the move must succeed. body: %s", string(raw))
		var got secretMoveEnvelope
		require.NoError(t, json.Unmarshal(raw, &got), "the envelope must decode. body: %s", string(raw))
		assert.Equal(t, secretName, got.Name)
		assert.Equal(t, "moved", got.Status)
		assert.Empty(t, got.Error, "a clean move reports no partial detail")

		// The value is on B and the source secret is gone.
		assert.Equal(t, "move-me-value", revealSecretViaAPI(t, b.MachineID, secretName))
		requireSecretNotListed(t, machineA, secretName, "a move removes the source secret")
	})

	// --- AC: a same-named target secret refuses; nothing is overwritten -
	t.Run("CollisionIs409", func(t *testing.T) {
		secretName := base + "-clash"
		cleanupSecret(secretName)
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "from-a"})
		createSecret(t, b.MachineID, secretCreateBody{Name: secretName, Value: "from-b"})

		status, raw := postSecretMove(t, machineA, secretName,
			secretMoveBody(t, b.MachineID, "copy"))
		require.Equalf(t, http.StatusConflict, status,
			"a same-named target secret must refuse the transfer. body: %s", string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		assert.Equal(t, "SECRET_ALREADY_EXISTS", errResp.Error.Code)

		// Machine B's secret keeps its own value.
		assert.Equal(t, "from-b", revealSecretViaAPI(t, b.MachineID, secretName))
	})

	// --- AC: the preview reports the dialog's facts ----------------------
	t.Run("PreviewReportsFacts", func(t *testing.T) {
		secretName := base + "-pv"
		quadlet := base + "-pvq"

		// The referencing-quadlet fact needs a quadlet file on the
		// source machine — the quadlet surface's systemd probe, gated
		// to this subtest alone.
		requireUserSystemd(t)

		cleanupSecret(secretName)
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "preview-value"})
		t.Cleanup(func() { forceDeleteQuadlet(t, machineA, quadlet+".container") })
		createQuadlet(t, machineA, quadletCreateBody{
			Name:     quadlet,
			Resource: "container",
			Content:  "[Container]\nImage=busybox\nSecret=" + secretName + "\n",
		})

		got := getSecretMovePreview(t, machineA, secretName, b.MachineID)
		assert.False(t, got.Collision, "machine B holds nothing of this run")
		require.Len(t, got.ReferencingQuadlets, 1)
		assert.Equal(t, quadlet+".container", got.ReferencingQuadlets[0].Name)

		// A same-named secret appearing on the target flips the
		// collision flag — the fact the dialog's confirm gate reads.
		createSecret(t, b.MachineID, secretCreateBody{Name: secretName, Value: "on-b"})
		got = getSecretMovePreview(t, machineA, secretName, b.MachineID)
		assert.True(t, got.Collision, "a same-named target secret is a collision")
	})
}

// Test_Integration_Secrets_Move_AccessControl pins the ACs the route
// middleware cannot express. The transfer's source gate is ADMIN — the
// value leaving the source is a reveal — so a write-level caller on both
// machines is refused with the plain 403 envelope and the value never
// lands on the target. And a caller holding admin on the source but only
// read on the target passes the preview's read gate yet is refused by the
// transfer's target write gate. Both gates fire before any podman call on
// the target, so this slice runs on every harness shape, systemd or not.
func Test_Integration_Secrets_Move_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerSecretsSweep()

	b := suite.RegisterSecondMachine(t)

	machineA := suite.MachineID
	base := "it-" + suite.RunID + "-secmvac"

	// --- AC: below admin on the source is 403, nothing moves -------------
	t.Run("BelowAdminSourceIs403", func(t *testing.T) {
		secretName := base + "-w"
		t.Cleanup(func() { deleteSecretBestEffort(t, machineA, secretName) })
		t.Cleanup(func() { deleteSecretBestEffort(t, b.MachineID, secretName) })
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "must-not-move"})

		// A write-level caller on both machines: enough for the route's
		// write gate, one level short of the reveal the transfer needs.
		client := belowAdminOnBothClient(t, b.MachineID)

		resp, err := client.Post(
			suite.ServerURL+"/api/machines/"+machineA+"/secrets/"+url.PathEscape(secretName)+"/move",
			"application/json", bytes.NewReader([]byte(secretMoveBody(t, b.MachineID, "copy"))))
		require.NoError(t, err, "POST /secrets/{id}/move must connect to the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusForbidden, resp.StatusCode,
			"a write-level caller must be refused. body: %s", string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)

		// The value never landed on the target — assertable only when
		// machine B holds its own podman store; on the shared-host
		// fallback the source's secret is visible on B by construction.
		if b.Independent {
			requireSecretNotListed(t, b.MachineID, secretName, "no value moved off the source")
		}
	})

	// --- AC: below write on the target is 403; the preview passes --------
	t.Run("TargetWriteDenied", func(t *testing.T) {
		secretName := base + "-t"
		t.Cleanup(func() { deleteSecretBestEffort(t, machineA, secretName) })
		t.Cleanup(func() { deleteSecretBestEffort(t, b.MachineID, secretName) })
		createSecret(t, machineA, secretCreateBody{Name: secretName, Value: "stays-put"})

		// A second, fully authenticated user holding admin on machine A
		// (the reveal gate passes) and read on machine B (below the
		// write the transfer needs) — the volume access slice's fixture
		// discipline.
		username := "it-" + suite.RunID + "-secmv-t"
		password := "it-" + suite.RunID + "-secmv-t-pw-" + uuid.New().String()[:8]
		userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
		require.NoError(t, err, "seed secret move target user")
		token, err := realLogin(suite.ServerURL, username, password)
		require.NoError(t, err, "login as secret move target user")
		reader := &http.Client{
			Timeout:   30 * time.Second,
			Transport: &bearerTransport{token: token, base: http.DefaultTransport},
		}

		// The suite user owns both machines; grant the reader admin on A
		// and read on B.
		for _, grant := range []struct {
			machineID  string
			permission string
		}{
			{machineID: machineA, permission: models.PermissionAdmin},
			{machineID: b.MachineID, permission: models.PermissionRead},
		} {
			grantBody, err := json.Marshal(map[string]string{
				"userId":     userID,
				"permission": grant.permission,
			})
			require.NoError(t, err)
			resp, err := suite.AuthenticatedClient().Post(
				suite.ServerURL+"/api/machines/"+grant.machineID+"/access",
				"application/json", bytes.NewReader(grantBody))
			require.NoError(t, err)
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.Equalf(t, http.StatusCreated, resp.StatusCode,
				"grant %s on %s: expected 201, got %d. body: %s",
				grant.permission, grant.machineID, resp.StatusCode, string(raw))
			t.Cleanup(func() {
				req, err := http.NewRequest(http.MethodDelete,
					suite.ServerURL+"/api/machines/"+grant.machineID+"/access/"+userID, nil)
				if err != nil {
					return
				}
				if resp, err := suite.AuthenticatedClient().Do(req); err == nil {
					resp.Body.Close()
				}
			})
		}

		// Positive control: the same caller reads machine A itself (no
		// podman involved), so the refusal below is the target gate, not
		// route-level source access.
		writerGet, err := reader.Get(suite.ServerURL + "/api/machines/" + machineA + "/")
		require.NoError(t, err)
		writerGet.Body.Close()
		require.Equal(t, http.StatusOK, writerGet.StatusCode,
			"the reader must pass machine A's subtree gate for the refusal below to mean target access")

		// The transfer across to machine B is refused with the standard
		// access error — nothing transferred, no error shape from the
		// podman layer.
		moveResp, err := reader.Post(
			suite.ServerURL+"/api/machines/"+machineA+"/secrets/"+url.PathEscape(secretName)+"/move",
			"application/json", bytes.NewReader([]byte(secretMoveBody(t, b.MachineID, "copy"))))
		require.NoError(t, err)
		defer moveResp.Body.Close()
		raw, _ := io.ReadAll(moveResp.Body)
		require.Equalf(t, http.StatusForbidden, moveResp.StatusCode,
			"read on the target must refuse the transfer. body: %s", string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
		// On the shared-host fallback the source's secret is visible on
		// B by construction, so the landing check needs independence.
		if b.Independent {
			requireSecretNotListed(t, b.MachineID, secretName, "no value landed on the target")
		}

		// The same caller's preview passes: read on the target suffices
		// there, and the source secret exists (the suite user's
		// fixture).
		previewResp, err := reader.Get(
			suite.ServerURL + "/api/machines/" + machineA + "/secrets/" + url.PathEscape(secretName) +
				"/move-preview?targetMachineId=" + b.MachineID)
		require.NoError(t, err)
		defer previewResp.Body.Close()
		raw, _ = io.ReadAll(previewResp.Body)
		require.Equalf(t, http.StatusOK, previewResp.StatusCode,
			"read on the target must satisfy the preview's gate. body: %s", string(raw))
	})
}
