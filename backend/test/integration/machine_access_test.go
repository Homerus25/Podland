//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/models"
)

// The no-access-user slice ("multi user test — can not access machines
// where the user has no access" in .scratch/Tests to issues.md): a
// SECOND, fully authenticated user who holds no machine_access row for
// the shared good machine tries every machine-scoped endpoint family
// and must be rejected.
//
// The user is real in every sense that matters: seeded through the
// app's seeding seam, logged in through POST /api/auth/login, and
// acting with its own JWT — the only thing distinguishing it from the
// owner is the absence of a machine_access row. That keeps the tests
// pinned to the authorization contract (machine access), not to
// authentication plumbing, which 080-06 already covers.
//
// Split into two test functions mirroring the enforcement layers:
//
//   - MachinesEndpointsRejectNoAccessUser pins the machines domain
//     (GET/PUT/DELETE /{id}, /access grant/list/revoke), which has
//     enforced access since 080-04.
//   - DownstreamEndpointsRejectNoAccessUser pins the downstream
//     domains that ride on a registered machine (containers, images,
//     quadlets, events). These are gated by the RequireMachineAccess
//     route middleware, which also gives unknown machine IDs a uniform
//     404 MACHINE_NOT_FOUND across those endpoints.

// ---------------------------------------------------------------------------
// No-access user fixture
// ---------------------------------------------------------------------------

// no-access user state, built lazily once per test binary. Seeding does
// a bcrypt hash, so the once guards against repeating it per test; the
// suite runs tests sequentially, so no extra locking is needed beyond
// sync.Once itself.
var (
	noAccessOnce   sync.Once
	noAccessClient *http.Client
	noAccessErr    error
)

// noAccessUserClient returns an *http.Client authenticated as a user
// that has access to NO machine. The user is seeded with the default
// "user" role and logged in through the real login endpoint, so every
// rejection observed through this client is an authorization decision,
// not an authentication failure.
func noAccessUserClient(t *testing.T) *http.Client {
	t.Helper()
	noAccessOnce.Do(func() {
		username := "it-" + suite.RunID + "-noaccess"
		password := "it-" + suite.RunID + "-noaccess-pw-" + uuid.New().String()[:8]
		if _, err := suite.SeedUserWithRole(username, password, models.RoleUser); err != nil {
			noAccessErr = fmt.Errorf("seed no-access user: %w", err)
			return
		}
		token, err := realLogin(suite.ServerURL, username, password)
		if err != nil {
			noAccessErr = fmt.Errorf("login as no-access user: %w", err)
			return
		}
		noAccessClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &bearerTransport{
				token: token,
				base:  http.DefaultTransport,
			},
		}
	})
	require.NoError(t, noAccessErr, "no-access user fixture must be buildable")
	require.NotNil(t, noAccessClient, "no-access client must be set on success")
	return noAccessClient
}

// apiErrorResponse mirrors the structured error body every handler
// emits via errors.RespondWithError: {"requestId":…, "error":{"code":…,
// "message":…}}. Only the fields the assertions consume are named.
type apiErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

// expectAPIError performs the request via client and asserts the
// response carries the expected HTTP status AND the machine-readable
// error code. Pinning both is the established tripwire style (see the
// CONNECTION_FAILED tripwire in machines_test.go): a status-only check
// would also pass on a 401 or 500 and mask the real surface.
func expectAPIError(t *testing.T, client *http.Client, method, url, wantBody string, wantStatus int, wantCode string) {
	t.Helper()

	var bodyReader io.Reader
	if wantBody != "" {
		bodyReader = strings.NewReader(wantBody)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	require.NoErrorf(t, err, "%s %s must be buildable", method, url)
	if wantBody != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	require.NoErrorf(t, err, "%s %s must reach the in-process server", method, url)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.Equalf(t, wantStatus, resp.StatusCode,
		"%s %s as no-access user: expected %d, got %d. body=%s",
		method, url, wantStatus, resp.StatusCode, string(raw))

	var errResp apiErrorResponse
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s %s: error body must decode as structured JSON. body=%s", method, url, string(raw))
	assert.Equalf(t, wantCode, errResp.Error.Code,
		"%s %s as no-access user: expected error code %s. body=%s",
		method, url, wantCode, string(raw))
}

// ---------------------------------------------------------------------------
// Machines domain endpoints
// ---------------------------------------------------------------------------

// Test_Integration_MachineAccess_MachinesEndpointsRejectNoAccessUser
// walks every machines endpoint that takes a machine ID as the
// no-access user and asserts 403 ACCESS_DENIED (and, for the list
// endpoint, that the inaccessible machine is filtered out of the
// response).
//
// Every subtest is a rejection, so the shared good machine is never
// mutated: the 403 fires before any write path runs. That is itself
// load-bearing — a regression that let the PUT/DELETE/access calls
// through would corrupt the fixture for every other slice.
func Test_Integration_MachineAccess_MachinesEndpointsRejectNoAccessUser(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := noAccessUserClient(t)

	machineURL := suite.ServerURL + "/api/machines/" + suite.MachineID + "/"

	t.Run("ListExcludesInaccessibleMachine", func(t *testing.T) {
		// Positive control first: the owner sees the machine, so its
		// absence from the no-access user's list is the access filter
		// at work — not a broken list endpoint returning [] for
		// everyone.
		require.True(t, machineListed(t, suite.MachineID),
			"owner must see the shared machine (precondition)")

		resp, err := client.Get(suite.ServerURL + "/api/machines/")
		require.NoError(t, err, "GET /api/machines/ must reach the in-process server")
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"list as no-access user: expected 200, got %d. body=%s", resp.StatusCode, string(body))

		var listed []machineDTO
		require.NoErrorf(t, json.Unmarshal(body, &listed),
			"list as no-access user: must decode as JSON array. body=%s", string(body))
		assert.False(t, containsMachine(listed, suite.MachineID),
			"no-access user's machine list must not contain the owner's machine. list=%+v", listed)
	})

	t.Run("GetMachineDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UpdateMachineDenied", func(t *testing.T) {
		// The Update handler decodes the body before the access check,
		// so the request must carry valid JSON to reach the 403.
		expectAPIError(t, client, http.MethodPut, machineURL, `{"name":"hijacked"}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("DeleteMachineDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, machineURL, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListAccessDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL+"access", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("GrantAccessDenied", func(t *testing.T) {
		// Granting to the suite's third-party user as a non-owner: the
		// handler's owner check fires before any grant is written.
		body := fmt.Sprintf(`{"userId":%q,"permission":"read"}`, suite.OtherUserID)
		expectAPIError(t, client, http.MethodPost, machineURL+"access", body,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("RevokeAccessDenied", func(t *testing.T) {
		url := machineURL + "access/" + suite.OtherUserID
		expectAPIError(t, client, http.MethodDelete, url, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	// The rejections must have been decisions, not side effects: the
	// owner still sees the machine and still holds the sole admin
	// grant after the no-access user's attempts.
	t.Run("OwnerFixtureIntactAfterRejections", func(t *testing.T) {
		assert.True(t, machineListed(t, suite.MachineID),
			"owner must still see the shared machine after no-access rejections")
		access := listAccess(t, suite.MachineID)
		assert.Len(t, access, 1,
			"owner must still hold the sole access grant. got: %+v", access)
	})
}

// containsMachine reports whether the decoded list contains the given
// machine ID. List filtering is the assertion target, so absence is
// checked on the no-access user's decoded list directly rather than
// through the owner-authenticated machineListed helper.
func containsMachine(listed []machineDTO, machineID string) bool {
	for _, m := range listed {
		if m.ID == machineID {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Downstream (machine-scoped) endpoints
// ---------------------------------------------------------------------------

// Test_Integration_MachineAccess_DownstreamEndpointsRejectNoAccessUser
// pins the authorization contract for every endpoint family that rides
// ON a registered machine: containers, images, quadlets, and events.
//
// Unlike the machines domain (080-04), these handlers historically
// trusted the JWT alone — a user with no machine_access row could list
// and mutate another user's containers, images, and quadlets through
// the owner's SSH connection. The RequireMachineAccess route middleware
// closes that gap for the whole /api/machines/{id}/** subtree in one
// seam.
//
// One read and one mutating endpoint per family is enough to prove the
// subtree wiring: the middleware gates the route before any handler or
// podman call runs, so a container ID that does not exist is never
// reached — the 403 fires first.
func Test_Integration_MachineAccess_DownstreamEndpointsRejectNoAccessUser(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := noAccessUserClient(t)

	machineURL := suite.ServerURL + "/api/machines/" + suite.MachineID + "/"

	t.Run("ListContainersDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL+"containers", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("StartContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, machineURL+"containers/no-such-container/start", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListImagesDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL+"images", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListQuadletsDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL+"quadlets", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("CreateQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, machineURL+"quadlets", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("MachineEventsDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, machineURL+"events", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	// Unknown machine IDs get a uniform 404 MACHINE_NOT_FOUND across
	// the subtree — the same answer the machines domain gives — instead
	// of the previous per-family split (500 INTERNAL_ERROR from
	// containers/images, 502 CONNECTION_FAILED from quadlets).
	t.Run("UnknownMachineIs404NotLeakedAs500", func(t *testing.T) {
		url := suite.ServerURL + "/api/machines/" + uuid.New().String() + "/containers"
		expectAPIError(t, client, http.MethodGet, url, "",
			http.StatusNotFound, "MACHINE_NOT_FOUND")
	})
}
