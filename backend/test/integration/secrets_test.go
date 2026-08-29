//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/require"
)

// The secrets slice (160-05): the four secret endpoints exercised
// end-to-end against real podman, plus the permission tripwires that
// complement the 160-00 level-aware access slice:
//
//	GET    /api/machines/{id}/secrets                 → 200 metadata list
//	POST   /api/machines/{id}/secrets                 → 201 (write gate)
//	GET    /api/machines/{id}/secrets/{secretId}      → 200 metadata | 200 reveal (admin only)
//	DELETE /api/machines/{id}/secrets/{secretId}      → 204 (write gate)
//
// Redaction discipline is the point of the reveal tests: the value
// travels exactly twice — in the create request and in an admin reveal
// response — and must come back byte-identical, whitespace and
// newlines included (the service never trims the value).
//
// Isolation model mirrors the quadlets CRUD slice: run-scoped names
// (it-<runID>-…), per-test t.Cleanup deletes via the API, and a
// suite-teardown sweep that removes any run-scoped secret an aborted
// test left behind.
//
// GAP (documented, not integration-covered): deleting a secret that a
// container references must answer 409 SECRET_IN_USE, but the
// container-create API (v1) cannot attach a secret to a container, so
// the in-use state is unreachable through the HTTP surface. The
// mapping is unit-covered instead
// (internal/domain/secrets/service_test.go TestSecretService_Delete_InUseConflict,
// TestSecretHandler_Delete_InUseSurfacesPodmanMessage).
func Test_Integration_Secrets_CRUD(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerSecretsSweep()

	machineID := suite.MachineID

	secretName := "it-" + suite.RunID + "-crud"
	// The whitespace and newlines are the secret: a service that trims
	// or normalizes the value fails the byte-identical reveal below.
	secretValue := "  s3cr3t value\nwith embedded\nnewlines and trailing spaces   "

	t.Cleanup(func() {
		deleteSecretBestEffort(t, machineID, secretName)
	})

	// --- Round trip: create → list → inspect → delete → gone ----------
	t.Run("CreateReturns201WithIDOnly", func(t *testing.T) {
		created := createSecret(t, machineID, secretCreateBody{Name: secretName, Value: secretValue})
		require.NotEmpty(t, created.ID, "create secret: response must carry the podman secret ID")
	})

	t.Run("ListReturnsCreatedSecretMetadata", func(t *testing.T) {
		requireSecretListed(t, machineID, secretName, "created secret must appear in the secrets list")
	})

	t.Run("InspectReturnsMetadataWithoutValue", func(t *testing.T) {
		got := getSecret(t, machineID, secretName, false)
		require.Equal(t, secretName, got.Name)
		require.NotEmpty(t, got.ID, "inspect must report the podman secret ID")
		require.NotEmpty(t, got.CreatedAt, "inspect must report createdAt")
		require.NotEmpty(t, got.UpdatedAt, "inspect must report updatedAt")
		// The `secretData` key must be absent entirely, not merely empty.
		require.NotContains(t, got.raw, "secretData",
			"metadata inspect must not include the secretData key at all")
	})

	if t.Failed() {
		return
	}

	t.Run("AdminRevealReturnsByteIdenticalValue", func(t *testing.T) {
		// The machine owner holds an admin access row (granted at
		// machine registration), so the suite client is the admin here.
		got := getSecret(t, machineID, secretName, true)
		require.NotNil(t, got.SecretData, "admin reveal must include the secretData field")
		require.Equal(t, secretValue, *got.SecretData,
			"revealed value must be byte-identical to what was created, whitespace and newlines included")
	})

	t.Run("RevealBySecretIDNotJustName", func(t *testing.T) {
		// {secretId} accepts a name OR an ID; resolve the ID via the
		// metadata inspect and reveal through it.
		metadata := getSecret(t, machineID, secretName, false)
		got := getSecret(t, machineID, metadata.ID, true)
		require.NotNil(t, got.SecretData, "admin reveal by ID must include the secretData field")
		require.Equal(t, secretValue, *got.SecretData, "reveal by podman secret ID must return the value")
	})

	t.Run("DeleteRemovesSecret", func(t *testing.T) {
		// Prove presence first so a broken list endpoint can't make the
		// post-delete absence check pass vacuously.
		requireSecretListed(t, machineID, secretName, "secret must appear in the list before delete")
		deleteSecret(t, machineID, secretName)
		requireSecretNotListed(t, machineID, secretName, "deleted secret must not appear in the secrets list")
		getSecretExpectError(t, machineID, secretName, false, http.StatusNotFound, "SECRET_NOT_FOUND")
	})

	t.Run("DeleteMissingSecretReturns404", func(t *testing.T) {
		deleteSecretExpectError(t, machineID, "it-"+suite.RunID+"-missing", http.StatusNotFound, "SECRET_NOT_FOUND")
	})

	t.Run("CreateDuplicateNameReturns409", func(t *testing.T) {
		dup := "it-" + suite.RunID + "-dup"
		t.Cleanup(func() { deleteSecretBestEffort(t, machineID, dup) })
		createSecret(t, machineID, secretCreateBody{Name: dup, Value: "x"})
		createSecretExpectError(t, machineID, secretCreateBody{Name: dup, Value: "x"},
			http.StatusConflict, "SECRET_ALREADY_EXISTS")
	})
}

// Test_Integration_Secrets_RevealIsAdminOnly pins the one admin-only
// operation in the whole API: ?reveal=true below admin is refused with
// 403 ACCESS_DENIED (not silently degraded to metadata) — and by
// construction the gate fires before the connection lease, so podman is
// never asked for the value.
func Test_Integration_Secrets_RevealIsAdminOnly(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerSecretsSweep()

	machineID := suite.MachineID
	secretName := "it-" + suite.RunID + "-reveal"
	t.Cleanup(func() { deleteSecretBestEffort(t, machineID, secretName) })
	createSecret(t, machineID, secretCreateBody{Name: secretName, Value: "reveal-me"})

	revealURL := suite.ServerURL + "/api/machines/" + machineID + "/secrets/" + url.PathEscape(secretName) + "?reveal=true"

	t.Run("WriteUserDenied", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionWrite)
		resp, err := client.Get(revealURL)
		require.NoError(t, err, "GET ?reveal=true must reach the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusForbidden, resp.StatusCode,
			"write-level reveal must be refused, got %d. body: %s", resp.StatusCode, string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		require.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
	})

	t.Run("ReadUserDenied", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		resp, err := client.Get(revealURL)
		require.NoError(t, err)
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusForbidden, resp.StatusCode,
			"read-level reveal must be refused, got %d. body: %s", resp.StatusCode, string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp))
		require.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
	})

	t.Run("MetadataInspectStillOpenToReadUser", func(t *testing.T) {
		// The negative reveal tests prove the gate; this is the positive
		// control that the gate is on the VALUE, not the endpoint: a
		// read-level caller keeps metadata inspect (no ?reveal param).
		client := levelUserClient(t, models.PermissionRead)
		resp, err := client.Get(suite.ServerURL + "/api/machines/" + machineID + "/secrets/" + url.PathEscape(secretName))
		require.NoError(t, err)
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"read-level metadata inspect must stay open, got %d. body: %s", resp.StatusCode, string(raw))
		require.NotContains(t, string(raw), "secretData")
	})
}

// Test_Integration_Secrets_WriteGatedMutations complements the 160-00
// slice with the secrets-specific tripwires: a read-level grantee gets
// 403 ACCESS_DENIED on create and delete. The 403 fires in middleware
// before the body is read, so the sensitive value never travels on a
// denied create.
func Test_Integration_Secrets_WriteGatedMutations(t *testing.T) {
	suite.SkipIfUnavailable(t)

	base := suite.ServerURL + "/api/machines/" + suite.MachineID
	client := levelUserClient(t, models.PermissionRead)

	t.Run("CreateSecretDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/secrets/",
			`{"name":"it-denied","value":"nope"}`, http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("DeleteSecretDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, base+"/secrets/no-such-secret", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers — following the quadlets slice conventions
// ---------------------------------------------------------------------------

// secretCreateBody mirrors the JSON-accepted fields of
// secrets.SecretCreateRequest that this slice populates. Kept as a
// local type so the integration package doesn't import the secrets
// domain package.
type secretCreateBody struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// secretCreateResponse mirrors secrets.SecretCreateResponse.
type secretCreateResponse struct {
	ID string `json:"id"`
}

// secretEntry mirrors secrets.SecretListResponse plus the optional
// reveal field. SecretData is captured as *string so an absent key is
// distinguishable from an empty value.
type secretEntry struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Driver    string            `json:"driver"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// secretInspectResponse mirrors secrets.SecretInspectResponse as the
// wire sees it, plus the raw body for absent-key assertions.
type secretInspectResponse struct {
	secretEntry
	SecretData *string `json:"secretData,omitempty"`
	raw        string
}

func secretsBaseURL(machineID string) string {
	return suite.ServerURL + "/api/machines/" + machineID + "/secrets/"
}

// createSecret POSTs /secrets and asserts 201 with an ID-only body.
func createSecret(t *testing.T, machineID string, body secretCreateBody) secretCreateResponse {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := suite.AuthenticatedClient().Post(secretsBaseURL(machineID), "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /secrets must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"create secret: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var created secretCreateResponse
	require.NoErrorf(t, json.Unmarshal(raw, &created),
		"create secret: response must decode. body: %s", string(raw))
	require.NotEmpty(t, created.ID, "create secret: response missing id")
	return created
}

// createSecretExpectError POSTs /secrets and asserts the structured
// error status + code.
func createSecretExpectError(t *testing.T, machineID string, body secretCreateBody, wantStatus int, wantCode string) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	resp, err := suite.AuthenticatedClient().Post(secretsBaseURL(machineID), "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"create secret: expected %d, got %d. body: %s", wantStatus, resp.StatusCode, string(raw))
	var errResp apiErrorResponse
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"create secret: error body must decode. body: %s", string(raw))
	require.Equal(t, wantCode, errResp.Error.Code,
		"create secret: error code must be %s", wantCode)
}

// listSecrets GETs /secrets and returns every metadata entry on the
// machine. Fails the test if any entry carries a secretData key.
func listSecrets(t *testing.T, machineID string) []secretEntry {
	t.Helper()
	resp, err := suite.AuthenticatedClient().Get(secretsBaseURL(machineID))
	require.NoError(t, err, "GET /secrets")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list secrets: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var result []secretEntry
	require.NoErrorf(t, json.Unmarshal(body, &result),
		"list secrets: response must decode as a JSON array. body: %s", string(body))
	require.NotContains(t, string(body), "secretData",
		"the secrets list must never carry a value")
	return result
}

func requireSecretListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	for _, entry := range listSecrets(t, machineID) {
		if entry.Name == name {
			return
		}
	}
	t.Fatalf("%s (not found among the machine's secrets)", msg)
}

func requireSecretNotListed(t *testing.T, machineID, name, msg string) {
	t.Helper()
	for _, entry := range listSecrets(t, machineID) {
		if entry.Name == name {
			t.Fatalf("%s", msg)
		}
	}
}

// getSecret GETs /secrets/{secretId} and decodes the inspect response.
func getSecret(t *testing.T, machineID, secretID string, reveal bool) secretInspectResponse {
	t.Helper()
	u := secretsBaseURL(machineID) + url.PathEscape(secretID) + "/"
	if reveal {
		u += "?reveal=true"
	}
	resp, err := suite.AuthenticatedClient().Get(u)
	require.NoError(t, err, "GET /secrets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"get secret: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var got secretInspectResponse
	require.NoErrorf(t, json.Unmarshal(body, &got),
		"get secret: response must decode. body: %s", string(body))
	got.raw = string(body)
	return got
}

// getSecretExpectError GETs /secrets/{secretId} and asserts the
// structured error status + code.
func getSecretExpectError(t *testing.T, machineID, secretID string, reveal bool, wantStatus int, wantCode string) {
	t.Helper()
	u := secretsBaseURL(machineID) + url.PathEscape(secretID) + "/"
	if reveal {
		u += "?reveal=true"
	}
	resp, err := suite.AuthenticatedClient().Get(u)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"get secret: expected %d, got %d. body: %s", wantStatus, resp.StatusCode, string(body))
	var errResp apiErrorResponse
	require.NoErrorf(t, json.Unmarshal(body, &errResp),
		"get secret: error body must decode. body: %s", string(body))
	require.Equal(t, wantCode, errResp.Error.Code)
}

// deleteSecret DELETEs /secrets/{secretId} and asserts 204 with an
// empty body.
func deleteSecret(t *testing.T, machineID, secretID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, secretsBaseURL(machineID)+url.PathEscape(secretID)+"/", nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /secrets/{id}")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode,
		"delete secret: expected 204, got %d. body: %s", resp.StatusCode, string(body))
	require.Empty(t, strings.TrimSpace(string(body)), "delete secret: 204 body must be empty")
}

// deleteSecretExpectError DELETEs /secrets/{secretId} and asserts the
// structured error status + code.
func deleteSecretExpectError(t *testing.T, machineID, secretID string, wantStatus int, wantCode string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, secretsBaseURL(machineID)+url.PathEscape(secretID)+"/", nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"delete secret: expected %d, got %d. body: %s", wantStatus, resp.StatusCode, string(body))
	var errResp apiErrorResponse
	require.NoErrorf(t, json.Unmarshal(body, &errResp),
		"delete secret: error body must decode. body: %s", string(body))
	require.Equal(t, wantCode, errResp.Error.Code)
}

// deleteSecretBestEffort removes a secret if it exists; used by
// t.Cleanup hooks after the test's own DELETE (404 is expected then).
func deleteSecretBestEffort(t *testing.T, machineID, secretID string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, secretsBaseURL(machineID)+url.PathEscape(secretID)+"/", nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

var registerSecretsSweepOnce sync.Once

// registerSecretsSweep removes any run-scoped secret left behind by an
// aborted test (panic, SIGINT before per-test cleanup fired).
// Best-effort: failures are logged by runCleanups, never fatal.
func registerSecretsSweep() {
	registerSecretsSweepOnce.Do(func() {
		runID := suite.RunID
		machineID := suite.MachineID
		suite.RegisterCleanup("sweep run-scoped secrets", func() error {
			sweepRunScopedSecrets(runID, machineID)
			return nil
		})
	})
}

// sweepRunScopedSecrets lists the machine's secrets via the API and
// deletes those whose name carries the run tag.
func sweepRunScopedSecrets(runID, machineID string) {
	client := suite.AuthenticatedClient()
	resp, err := client.Get(secretsBaseURL(machineID))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var entries []secretEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.Contains(entry.Name, runID) {
			continue
		}
		req, err := http.NewRequest(http.MethodDelete,
			secretsBaseURL(machineID)+url.PathEscape(entry.Name)+"/", nil)
		if err != nil {
			continue
		}
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}
}
