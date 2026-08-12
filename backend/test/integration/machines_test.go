//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// Test_Integration_Machines_CRUDAccessTripwire is the machines CRUD +
// access-control vertical slice for the real-podman integration suite
// (PRD 080-04). It registers a SECOND, throwaway machine pointing at an
// unreachable target (127.0.0.1:1) and uses it to exercise PUT, access
// grant/revoke/list, and DELETE — without disturbing the shared good
// machine registered during harness setup.
//
// Why a throwaway: every mutation in this slice (PUT, access, DELETE)
// is a per-machine DB row, not a podman operation. Running them against
// the shared good machine would change its name/hostname and leave
// grantee rows behind, breaking the contract that subsequent slices
// (containers, images, …) see the machine as TestMain registered it.
// The bad-host machine is the only one this slice mutates, and it is
// deleted at the end of the run.
//
// Tripwire (gap #1, issue 080-05): with the current code,
// MachineService.Create performs NO connection test, so registering the
// bad-host machine returns 201 and the failure surfaces only at the
// first podman-touching call. This slice pins that behavior so the test
// documents reality and flips naturally when 080-05 lands — at which
// point the "first podman call fails" assertion is removed and the
// registration assertion becomes 400 CONNECTION_FAILED.
//
// Isolation model (per the master PRD's "run-scoped tag + per-test
// cleanup + suite sweep" ruling):
//
//   - The throwaway machine is named with the run-scoped prefix
//     (it-<runID>-badhost) so the suite sweep can find and delete any
//     leftover if a test aborts before its own t.Cleanup fires.
//   - t.Cleanup force-deletes the throwaway via the API as the primary
//     cleanup path. The sweep is the backstop.
//   - No podman resources are created or mutated by this slice, so
//     there is no run-tag labelling and no podman CLI sweep.
func Test_Integration_Machines_CRUDAccessTripwire(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerMachinesSweep()

	// --- Shared throwaway: the bad-host machine ----------------------
	//
	// 127.0.0.1:1 is the canonical "nobody's listening" target: TCP
	// connect refuses immediately (ECONNREFUSED), so the SSH dial fails
	// fast instead of hanging on a connect timeout. The SSH key is a
	// freshly-generated RSA PEM — syntactically valid so
	// MachineService.Create's IsValidPEM check passes, but it won't
	// authenticate anywhere. That's the point: registration must not
	// depend on the key being usable, only on it being well-formed.
	badHost := machineRegistration{
		Name:    "it-" + suite.RunID + "-badhost",
		Host:    "127.0.0.1",
		Port:    1,
		SSHUser: "nobody",
		Socket:  "/run/podman/podman.sock",
		SSHKey:  testutils.GenerateValidPEMKey(),
	}

	// badHostID is captured by the RegisterReturns201 subtest and read
	// by every later subtest. The whole slice operates on exactly one
	// throwaway machine.
	var badHostID string

	// Snapshot the shared good machine's fields BEFORE any mutation in
	// this slice. The DeleteRemovesThrowawaySharedUntouched subtest
	// compares against this snapshot so "untouched" means field-equal,
	// not just "still listed" — which would mask a regression that
	// mutated the shared machine's name/hostname mid-test. Captured
	// here (not inside the delete subtest) so the snapshot is taken
	// before the PUT in updateMachineCRUD runs, which is the earliest
	// mutation this slice makes.
	goodMachineBefore := getMachine(t, suite.MachineID)

	// Per-test cleanup: DELETE the throwaway machine via the API if it
	// was created and not already deleted by the DeleteRemovesMachine
	// subtest. Best-effort; the sweep is the abort backstop.
	t.Cleanup(func() {
		if badHostID == "" {
			return
		}
		delURL := suite.ServerURL + "/api/machines/" + badHostID + "/"
		req, err := http.NewRequest(http.MethodDelete, delURL, nil)
		if err != nil {
			return
		}
		resp, err := suite.AuthenticatedClient().Do(req)
		if err != nil {
			t.Logf("cleanup: DELETE throwaway machine best-effort failed: %v", err)
			return
		}
		resp.Body.Close()
	})

	// --- AC #1 (tripwire, gap-#1): registration returns 201 ----------
	//
	// Pinning current behavior. MachineService.Create does NOT probe
	// the target, so a bad host sails through and the failure surfaces
	// later. When 080-05 lands this assertion flips to:
	//
	//	assertStatusCode(t, resp, http.StatusBadRequest, "register bad-host machine")
	//	assertErrorCode(t, body, "CONNECTION_FAILED")
	//
	// and the FirstPodmanCallFails subtest below is removed.
	t.Run("RegisterReturns201_Tripwire_Gap1", func(t *testing.T) {
		var err error
		badHostID, err = registerMachine(suite.ServerURL, suite.Token, badHost)
		require.NoError(t, err, "register bad-host machine must succeed (current gap-#1 behavior)")
		require.NotEmpty(t, badHostID, "register must return a machine id")
		t.Logf("tripwire (gap-#1): bad-host machine registered as %s with 201 — MachineService.Create performs no connection test", badHostID)
	})
	if t.Failed() {
		return
	}

	// --- AC #2 (tripwire, gap-#1): first podman call fails ----------
	//
	// Documents where the bad-host failure actually surfaces today: not
	// at registration, but at the first podman-touching call. We hit
	// GET /containers because it's the cheapest podman probe and the
	// same one the tracer bullet uses — so the assertion is directly
	// comparable to the happy path. The container List handler
	// (containers/handler.go) returns 500 via ErrInternalError when
	// connManager.GetConnection fails; errors.RespondWithError then
	// sanitises the 5xx body to a generic
	// {"error":{"code":"INTERNAL_ERROR","message":"An unexpected error occurred"}}
	// so the underlying SSH cause ("dial tcp ... connection refused")
	// surfaces only in the server log, NOT in the HTTP body. Pinning
	// the status AND the INTERNAL_ERROR code is what "documents where
	// the failure surfaces today" means in this codebase; a bare
	// `!= 200` would also pass on a 404 or 401 and mask the real
	// surface, and asserting the SSH cause lives in the body would
	// fail (it doesn't — by design).
	//
	// Removed when 080-05 lands: a connection-test at registration
	// means the bad-host machine never gets created, so there is no
	// machine to probe.
	t.Run("FirstPodmanCallFails_Tripwire_Gap1", func(t *testing.T) {
		url := suite.ServerURL + "/api/machines/" + badHostID + "/containers/"
		resp, err := suite.AuthenticatedClient().Get(url)
		require.NoError(t, err, "GET containers must reach the in-process server")
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equalf(t, http.StatusInternalServerError, resp.StatusCode,
			"tripwire (gap-#1): bad-host machine's first podman-touching call must surface as 500 (ErrInternalError from connManager.GetConnection). got %d. body=%s",
			resp.StatusCode, string(body))
		// The 5xx body is sanitised by errors.RespondWithError to a
		// generic INTERNAL_ERROR — the underlying SSH failure lives
		// only in the server log. Pin the code so a future "200 +
		// swallow" or a non-500 error-class regression fails the test.
		assert.Contains(t, string(body), "INTERNAL_ERROR",
			"tripwire (gap-#1): 500 body must carry the INTERNAL_ERROR code (the underlying SSH cause is server-log only). body=%s", string(body))
		t.Logf("tripwire (gap-#1): first podman-touching call failed as expected — status=%d body=%s (the underlying SSH dial error is in the server log only)",
			resp.StatusCode, string(body))
	})
	if t.Failed() {
		return
	}

	// --- AC #3: PUT updates name/description/hostname; GET reflects -
	updateMachineCRUD(t, badHostID)
	if t.Failed() {
		return
	}

	// --- AC #4: access grant/list/revoke; self-revoke rejected ------
	exerciseAccessCRUD(t, badHostID)
	if t.Failed() {
		return
	}

	// --- AC #5: DELETE removes the throwaway; shared good machine untouched
	//
	// Inlined (not factored out like updateMachineCRUD/exerciseAccessCRUD)
	// because this subtest needs three pieces of parent state — the
	// throwaway ID, a writable handle to null after delete, and the
	// good-machine snapshot taken before any mutation in this slice.
	// A helper signature for all three would be the data-clump smell
	// the review caught; the containers slice inlines its delete
	// subtest for the same reason.
	t.Run("DeleteRemovesThrowawaySharedUntouched", func(t *testing.T) {
		// Prove the shared good machine is present BEFORE the delete,
		// so the post-delete "still present" assertion can't pass
		// vacuously (e.g. if list were broken and always returned []).
		require.True(t, machineListed(t, suite.MachineID),
			"shared good machine must be listed before delete (precondition)")

		// Prove the throwaway is also present before delete, so the
		// post-delete absence check is meaningful.
		require.True(t, machineListed(t, badHostID),
			"throwaway machine must be listed before delete (precondition)")

		// DELETE the throwaway.
		url := suite.ServerURL + "/api/machines/" + badHostID + "/"
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		require.NoError(t, err)
		resp, err := suite.AuthenticatedClient().Do(req)
		require.NoError(t, err, "DELETE /api/machines/{id} must reach the in-process server")
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusNoContent, resp.StatusCode,
			"delete throwaway: expected 204, got %d. body: %s", resp.StatusCode, string(body))

		// Null the caller's handle so t.Cleanup doesn't retry the
		// delete on an already-removed machine.
		badHostID = ""

		// Throwaway is gone from the list…
		assert.False(t, machineListed(t, badHostID),
			"throwaway machine must not appear in the list after delete")

		// …and the shared good machine is still there, untouched.
		assert.True(t, machineListed(t, suite.MachineID),
			"shared good machine must still appear in the list after throwaway delete")
		// Field-equal check against the snapshot taken at the top of
		// the test — "untouched" means name/hostname/socket/etc. are
		// byte-equal, not just "still resolvable". A regression that
		// PUT the throwaway's fields onto the wrong row would slip a
		// presence-only check.
		good := getMachine(t, suite.MachineID)
		assert.Equal(t, goodMachineBefore, good,
			"shared good machine must be field-equal before vs after throwaway delete (name/hostname/etc. must not change)")
	})
}

// ---------------------------------------------------------------------------
// AC #3: PUT updates name/description/hostname
// ---------------------------------------------------------------------------

// updateMachineCRUD PUTs a new name/description/hostname onto the
// throwaway machine and verifies each field round-trips through a
// subsequent GET. Only the three AC-named fields are exercised — the
// Update handler also accepts port/SSH user/key/socket, but touching
// those here would obscure the "name/description/hostname" focus and
// risk changing the SSH target mid-test (irrelevant to the bad-host
// machine, but a footgun if this slice is ever reused on a live one).
//
// The handler treats "" as "leave unchanged" for each string field, so
// all three must be non-empty for the round-trip assertion to be
// meaningful.
func updateMachineCRUD(t *testing.T, machineID string) {
	t.Run("UpdateNameDescriptionHostname", func(t *testing.T) {
		// Capture pre-update state so the assertion can prove the
		// fields actually changed, not just that GET returns the new
		// values. A test that doesn't compare against the prior state
		// would pass vacuously if PUT were a no-op and GET returned
		// whatever it was handed.
		before := getMachine(t, machineID)

		req := updateMachineRequest{
			Name:        "it-" + suite.RunID + "-badhost-renamed",
			Description: "throwaway bad-host machine for 080-04 (updated)",
			Hostname:    "192.0.2.123", // TEST-NET-1 — guaranteed unroutable
			// Echo the SSH identity so the handler's non-empty checks
			// for port/SSH user/key/socket don't fire; we are NOT
			// changing them. Re-using the original values keeps the
			// update focused on the three AC fields.
			Port:             before.Port,
			SSHUsername:      before.SSHUsername,
			PodmanSocketPath: before.PodmanSocketPath,
			// SSHPrivateKey intentionally omitted: passing the existing
			// key would force a re-encrypt + fingerprint change, which
			// the AC doesn't ask for and would couple this assertion
			// to encryption internals.
		}

		updated := updateMachine(t, machineID, req)

		// Each field must match what we PUT AND differ from before —
		// the "differs from before" half is what makes the assertion
		// sensitive to a no-op PUT.
		assert.Equal(t, req.Name, updated.Name, "PUT must change name")
		assert.NotEqual(t, before.Name, updated.Name, "name must differ from before update")
		assert.Equal(t, req.Description, updated.Description, "PUT must change description")
		assert.NotEqual(t, before.Description, updated.Description, "description must differ from before update")
		assert.Equal(t, req.Hostname, updated.Hostname, "PUT must change hostname")
		assert.NotEqual(t, before.Hostname, updated.Hostname, "hostname must differ from before update")

		// Re-GET to prove the update was persisted, not just echoed
		// in the PUT response.
		refetched := getMachine(t, machineID)
		assert.Equal(t, updated, refetched, "GET after PUT must reflect the persisted state")
	})
}

// updateMachineRequest mirrors the JSON-accepted fields of
// machines.CreateMachineRequest. The Update handler reuses that request
// type and treats "" / 0 as "leave unchanged" for each field. Kept as a
// local type so the integration package stays decoupled from the
// machines domain package (matching the images/containers slice style).
type updateMachineRequest struct {
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	Hostname         string `json:"hostname,omitempty"`
	Port             int    `json:"port,omitempty"`
	SSHUsername      string `json:"sshUsername,omitempty"`
	SSHPrivateKey    string `json:"sshPrivateKey,omitempty"`
	PodmanSocketPath string `json:"podmanSocketPath,omitempty"`
}

// updateMachine PUTs /api/machines/{id}/ and returns the decoded
// post-update machine state. Asserts 200 (the Update handler writes
// StatusOK implicitly by writing the body without an explicit code).
func updateMachine(t *testing.T, machineID string, req updateMachineRequest) machineDTO {
	t.Helper()
	payload, err := json.Marshal(req)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/"
	httpReq, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := suite.AuthenticatedClient().Do(httpReq)
	require.NoError(t, err, "PUT /api/machines/{id} must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"update machine: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	return decodeMachine(t, body, "update machine")
}

// ---------------------------------------------------------------------------
// AC #4: access grant/list/revoke; self-revoke rejected
// ---------------------------------------------------------------------------

// accessResponse mirrors the JSON the GrantAccess/ListAccess handlers
// emit. Both use the same shape, so one local type covers POST and GET.
type accessResponse struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
	GrantedAt  string `json:"grantedAt"`
}

// exerciseAccessCRUD walks the full grant → list → revoke lifecycle on
// the throwaway machine, plus the owner self-revoke rejection.
//
// All three permissions (read/write/admin) are exercised via
// grant→list→revoke cycles on the same grantee. The handler's
// "existingAccess" check means a grantee can hold only one permission
// at a time, so we cycle: grant read, assert, revoke; grant write,
// assert, revoke; grant admin, assert, revoke. This proves the endpoint
// accepts each permission value — not just one — without needing three
// different grantees.
//
// The grantee is suite.OtherUserID (a real user row seeded in TestMain
// to satisfy the machine_access.user_id FK). The owner is the suite's
// authenticated user throughout — only the owner can grant/revoke.
func exerciseAccessCRUD(t *testing.T, machineID string) {
	t.Run("GrantListRevokeSelfRevokeRejected", func(t *testing.T) {
		// Baseline: only the owner holds admin (created at machine
		// registration time). Establishing this up front makes the
		// post-grant assertion ("now there are two") meaningful.
		initial := listAccess(t, machineID)
		require.Len(t, initial, 1,
			"baseline access list must contain only the owner's admin grant. got: %+v", initial)
		assert.Equal(t, suite.OwnerUserID, initial[0].UserID,
			"baseline access list must be the owner")
		assert.Equal(t, "admin", initial[0].Permission,
			"baseline access list must be owner's admin grant")

		// Cycle through all three permission values to prove the
		// endpoint accepts each one (AC: "grants read/write/admin").
		for _, perm := range []string{"read", "write", "admin"} {
			t.Run("grant_"+perm, func(t *testing.T) {
				grantAccess(t, machineID, suite.OtherUserID, perm)

				granted := listAccess(t, machineID)
				require.Len(t, granted, 2,
					"after granting %s, access list must contain owner + grantee. got: %+v", perm, granted)
				grantee := findAccess(granted, suite.OtherUserID)
				require.NotNil(t, grantee,
					"grantee must appear in access list after grant(%s). list: %+v", perm, granted)
				assert.Equal(t, perm, grantee.Permission,
					"granted permission must be %s. list: %+v", perm, granted)
			})
			if t.Failed() {
				return
			}

			t.Run("revoke_after_"+perm, func(t *testing.T) {
				revokeAccess(t, machineID, suite.OtherUserID)

				after := listAccess(t, machineID)
				require.Len(t, after, 1,
					"after revoke, access list must contain only the owner again. got: %+v", after)
				require.Nil(t, findAccess(after, suite.OtherUserID),
					"grantee must not appear in access list after revoke. list: %+v", after)
			})
			if t.Failed() {
				return
			}
		}

		// Owner self-revoke is rejected. The handler returns 400 with
		// code CANNOT_REVOKE_OWNER — pinning both the status and the
		// machine-readable code so a future "200 + ignore" regression
		// fails the test.
		//
		// "Self-revoke" means revoking the OWNER, not the grantee.
		// We DELETE /access/{ownerID} directly — no prior grant is
		// needed because the owner already holds admin from machine
		// registration. The handler's checks run in order: machine
		// exists → caller is owner → target is not the owner → delete.
		// The third check is what we are exercising here.
		t.Run("SelfRevokeRejected", func(t *testing.T) {
			url := suite.ServerURL + "/api/machines/" + machineID + "/access/" + suite.OwnerUserID
			req, err := http.NewRequest(http.MethodDelete, url, nil)
			require.NoError(t, err)
			resp, err := suite.AuthenticatedClient().Do(req)
			require.NoError(t, err, "DELETE /access/{owner} must reach the in-process server")
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			assert.Equalf(t, http.StatusBadRequest, resp.StatusCode,
				"owner self-revoke must be rejected with 400. got %d. body: %s",
				resp.StatusCode, string(body))
			assert.Contains(t, string(body), "CANNOT_REVOKE_OWNER",
				"owner self-revoke must surface the CANNOT_REVOKE_OWNER code. body: %s", string(body))

			// Self-revoke must not have removed anything: the owner
			// is still on the list.
			after := listAccess(t, machineID)
			require.NotNil(t, findAccess(after, suite.OwnerUserID),
				"owner must still appear in access list after rejected self-revoke. list: %+v", after)
		})
	})
}

// grantAccess POSTs /access for (userID, permission) and asserts 201.
// Used inside the per-permission cycle in exerciseAccessCRUD.
func grantAccess(t *testing.T, machineID, userID, permission string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"userId":     userID,
		"permission": permission,
	})
	require.NoError(t, err)
	url := suite.ServerURL + "/api/machines/" + machineID + "/access"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /access must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"grant access(%s): expected 201, got %d. body: %s",
		permission, resp.StatusCode, string(body))
}

// revokeAccess DELETEs /access/{userID} and asserts 204.
func revokeAccess(t *testing.T, machineID, userID string) {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/access/" + userID
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE /access/{userId} must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode,
		"revoke access: expected 204, got %d. body: %s", resp.StatusCode, string(body))
}

// findAccess returns the access entry for userID, or nil if absent.
// Used by the per-permission grant/revoke cycle to locate the grantee
// without assuming list ordering (ListByMachine orders by granted_at
// DESC, but relying on that would couple the assertion to repo
// internals).
func findAccess(entries []accessResponse, userID string) *accessResponse {
	for i := range entries {
		if entries[i].UserID == userID {
			return &entries[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// HTTP helpers — shared across the subtests
// ---------------------------------------------------------------------------

// machineDTO mirrors the fields of machines.MachineResponse that this
// slice consumes. Kept as a local type so the integration package
// doesn't import the machines domain package (matching the
// images/containers slice style).
type machineDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Hostname          string `json:"hostname"`
	Port              int    `json:"port"`
	SSHUsername       string `json:"sshUsername"`
	SSHKeyFingerprint string `json:"sshKeyFingerprint"`
	PodmanSocketPath  string `json:"podmanSocketPath"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

// getMachine GETs /api/machines/{id}/ and returns the decoded machine.
// Asserts 200 (the Get handler writes StatusOK implicitly).
func getMachine(t *testing.T, machineID string) machineDTO {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /api/machines/{id} must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"get machine: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	return decodeMachine(t, body, "get machine")
}

// listMachines GETs /api/machines/ and returns the decoded list. Used
// by the post-delete presence/absence checks.
func listMachines(t *testing.T) []machineDTO {
	t.Helper()
	url := suite.ServerURL + "/api/machines/"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /api/machines/ must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list machines: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var out []machineDTO
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"list machines: response must decode as a JSON array. body: %s", string(body))
	return out
}

// machineListed reports whether the given machine ID appears in the
// owner's machine list. Used to prove presence/absence without caring
// about ordering or other entries.
func machineListed(t *testing.T, machineID string) bool {
	t.Helper()
	for _, m := range listMachines(t) {
		if m.ID == machineID {
			return true
		}
	}
	return false
}

// listAccess GETs /api/machines/{id}/access and returns the decoded
// list of access entries.
func listAccess(t *testing.T, machineID string) []accessResponse {
	t.Helper()
	url := suite.ServerURL + "/api/machines/" + machineID + "/access"
	resp, err := suite.AuthenticatedClient().Get(url)
	require.NoError(t, err, "GET /access must reach the in-process server")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list access: expected 200, got %d. body: %s", resp.StatusCode, string(body))
	var out []accessResponse
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"list access: response must decode as a JSON array. body: %s", string(body))
	return out
}

// decodeMachine unmarshals a machine JSON object, attaching the given
// label to the failure message so the caller's context (PUT vs GET)
// shows up in the assertion error.
func decodeMachine(t *testing.T, body []byte, label string) machineDTO {
	t.Helper()
	var m machineDTO
	require.NoErrorf(t, json.Unmarshal(body, &m),
		"%s: response must decode as a MachineResponse. body: %s", label, string(body))
	if m.ID == "" {
		t.Logf("warning: %s decoded a machine with empty id. body: %s", label, string(body))
	}
	return m
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

// registerMachinesSweepOnce ensures the machines suite-sweep cleanup
// is registered at most once per process. The sweep queries the
// in-process backend for any machine whose name mentions this run's
// RunID — i.e. garbage a test left behind when its own t.Cleanup
// didn't fire (panic, SIGINT before the per-test cleanup registered, …)
// — and force-deletes it via the API.
//
// Registered at the top of the machines test, so it is in place before
// any garbage is created.
//
// We sweep via the API (not direct DB access) on purpose: the
// integration package deliberately does NOT reach into App's *sql.DB
// (see main_test.go), and the API path is the same one production
// cleanups would use.
//
// Best-effort: failures are logged by runCleanups, never fatal.
var registerMachinesSweepOnce sync.Once

func registerMachinesSweep() {
	registerMachinesSweepOnce.Do(func() {
		runID := suite.RunID
		token := suite.Token
		base := suite.ServerURL
		suite.RegisterCleanup("sweep run-scoped machines", func() error {
			sweepRunScopedMachines(base, token, runID)
			return nil
		})
	})
}

// sweepRunScopedMachines lists machines via the API and deletes any
// whose name mentions runID. Skips silently if the suite is
// unavailable (base == "" — the bearerTransport would fail anyway, but
// bailing here keeps teardown quiet on a setup that never booted).
func sweepRunScopedMachines(base, token, runID string) {
	if base == "" || token == "" {
		return
	}
	url := base + "/api/machines/"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var ms []machineDTO
	if err := json.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return
	}
	for _, m := range ms {
		// Match by RunID substring rather than full name so the sweep
		// also catches the renamed throwaway (UpdateNameDescriptionHostname
		// changes the suffix but keeps the it-<runID>- prefix).
		if m.ID == "" || !strings.Contains(m.Name, runID) {
			continue
		}
		delReq, err := http.NewRequest(http.MethodDelete, base+"/api/machines/"+m.ID+"/", nil)
		if err != nil {
			continue
		}
		delReq.Header.Set("Authorization", "Bearer "+token)
		delResp, err := http.DefaultClient.Do(delReq)
		if err != nil {
			continue
		}
		delResp.Body.Close()
	}
}
