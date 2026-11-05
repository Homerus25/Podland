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

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The no-access-user slice ("multi user test — can not access hosts
// where the user has no access" in .scratch/Tests to issues.md): a
// SECOND, fully authenticated user who holds no host_access row for
// the shared good host tries every host-scoped endpoint family
// and must be rejected.
//
// The user is real in every sense that matters: seeded through the
// app's seeding seam, logged in through POST /api/auth/login, and
// acting with its own JWT — the only thing distinguishing it from the
// owner is the absence of a host_access row. That keeps the tests
// pinned to the authorization contract (host access), not to
// authentication plumbing, which 080-06 already covers.
//
// Split into two test functions mirroring the enforcement layers:
//
//   - HostsEndpointsRejectNoAccessUser pins the hosts domain
//     (GET/PUT/DELETE /{id}, /access grant/list/revoke), which has
//     enforced access since 080-04.
//   - DownstreamEndpointsRejectNoAccessUser pins the downstream
//     domains that ride on a registered host (containers, images,
//     quadlets). These are gated by the RequireHostAccess
//     route middleware, which also gives unknown host IDs a uniform
//     404 HOST_NOT_FOUND across those endpoints.

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
	noAccessUserID string
	noAccessErr    error
)

// noAccessUserClient returns an *http.Client authenticated as a user
// that has access to NO host. The user is seeded with the default
// "user" role and logged in through the real login endpoint, so every
// rejection observed through this client is an authorization decision,
// not an authentication failure.
func noAccessUserClient(t *testing.T) *http.Client {
	t.Helper()
	noAccessOnce.Do(func() {
		username := "it-" + suite.RunID + "-noaccess"
		password := "it-" + suite.RunID + "-noaccess-pw-" + uuid.New().String()[:8]
		userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
		if err != nil {
			noAccessErr = fmt.Errorf("seed no-access user: %w", err)
			return
		}
		noAccessUserID = userID
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
// response carries the expected HTTP status AND the parseable
// error code. Pinning both is the established tripwire style (see the
// CONNECTION_FAILED tripwire in hosts_test.go): a status-only check
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
// Hosts domain endpoints
// ---------------------------------------------------------------------------

// Test_Integration_HostAccess_HostsEndpointsRejectNoAccessUser
// walks every hosts endpoint that takes a host ID as the
// no-access user and asserts 403 ACCESS_DENIED (and, for the list
// endpoint, that the inaccessible host is filtered out of the
// response).
//
// Every subtest is a rejection, so the shared good host is never
// mutated: the 403 fires before any write path runs. That is itself
// load-bearing — a regression that let the PUT/DELETE/access calls
// through would corrupt the fixture for every other slice.
func Test_Integration_HostAccess_HostsEndpointsRejectNoAccessUser(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := noAccessUserClient(t)

	hostURL := suite.ServerURL + "/api/hosts/" + suite.HostID + "/"

	t.Run("ListExcludesInaccessibleHost", func(t *testing.T) {
		// Positive control first: the owner sees the host, so its
		// absence from the no-access user's list is the access filter
		// at work — not a broken list endpoint returning [] for
		// everyone.
		require.True(t, hostListed(t, suite.HostID),
			"owner must see the shared host (precondition)")

		resp, err := client.Get(suite.ServerURL + "/api/hosts/")
		require.NoError(t, err, "GET /api/hosts/ must reach the in-process server")
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"list as no-access user: expected 200, got %d. body=%s", resp.StatusCode, string(body))

		var listed []hostDTO
		require.NoErrorf(t, json.Unmarshal(body, &listed),
			"list as no-access user: must decode as JSON array. body=%s", string(body))
		assert.False(t, containsHost(listed, suite.HostID),
			"no-access user's host list must not contain the owner's host. list=%+v", listed)
	})

	t.Run("GetHostDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, hostURL, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("UpdateHostDenied", func(t *testing.T) {
		// The Update handler decodes the body before the access check,
		// so the request must carry valid JSON to reach the 403.
		expectAPIError(t, client, http.MethodPut, hostURL, `{"name":"hijacked"}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("DeleteHostDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, hostURL, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListAccessDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, hostURL+"access", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("GrantAccessDenied", func(t *testing.T) {
		// Granting to the suite's third-party user as a non-owner: the
		// handler's owner check fires before any grant is written.
		body := fmt.Sprintf(`{"userId":%q,"permission":"read"}`, suite.OtherUserID)
		expectAPIError(t, client, http.MethodPost, hostURL+"access", body,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("RevokeAccessDenied", func(t *testing.T) {
		url := hostURL + "access/" + suite.OtherUserID
		expectAPIError(t, client, http.MethodDelete, url, "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	// The rejections must have been decisions, not side effects: the
	// owner still sees the host, still holds the admin grant, and —
	// the point of this tripwire — the no-access user's attempts wrote
	// no grant for themselves. The owner's is NOT the sole grant on the
	// shared host: the binary-wide read/write level fixtures
	// (host_permission_test.go) hold legitimately granted access.
	t.Run("OwnerFixtureIntactAfterRejections", func(t *testing.T) {
		assert.True(t, hostListed(t, suite.HostID),
			"owner must still see the shared host after no-access rejections")
		access := listAccess(t, suite.HostID)
		ownerGrant := findAccess(access, suite.OwnerUserID)
		if assert.NotNil(t, ownerGrant,
			"owner must still hold an access grant. got: %+v", access) {
			assert.Equal(t, "admin", ownerGrant.Permission,
				"owner grant must still be admin after no-access rejections. got: %+v", access)
		}
		assert.Nil(t, findAccess(access, noAccessUserID),
			"the no-access user must hold no grant after its rejected attempts. got: %+v", access)
	})
}

// containsHost reports whether the decoded list contains the given
// host ID. List filtering is the assertion target, so absence is
// checked on the no-access user's decoded list directly rather than
// through the owner-authenticated hostListed helper.
func containsHost(listed []hostDTO, hostID string) bool {
	for _, m := range listed {
		if m.ID == hostID {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Downstream (host-scoped) endpoints
// ---------------------------------------------------------------------------

// Test_Integration_HostAccess_DownstreamEndpointsRejectNoAccessUser
// pins the authorization contract for every endpoint family that rides
// ON a registered host: containers, images, and quadlets.
//
// Unlike the hosts domain (080-04), these handlers historically
// trusted the JWT alone — a user with no host_access row could list
// and mutate another user's containers, images, and quadlets through
// the owner's SSH connection. The RequireHostAccess route middleware
// closes that gap for the whole /api/hosts/{id}/** subtree in one
// seam.
//
// One read and one mutating endpoint per family is enough to prove the
// subtree wiring: the middleware gates the route before any handler or
// podman call runs, so a container ID that does not exist is never
// reached — the 403 fires first.
func Test_Integration_HostAccess_DownstreamEndpointsRejectNoAccessUser(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := noAccessUserClient(t)

	hostURL := suite.ServerURL + "/api/hosts/" + suite.HostID + "/"

	t.Run("ListContainersDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, hostURL+"containers", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("StartContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, hostURL+"containers/no-such-container/start", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListImagesDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, hostURL+"images", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ListQuadletsDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodGet, hostURL+"quadlets", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("CreateQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, hostURL+"quadlets", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})

	// Unknown host IDs get a uniform 404 HOST_NOT_FOUND across
	// the subtree — the same answer the hosts domain gives — instead
	// of the previous per-family split (500 INTERNAL_ERROR from
	// containers/images, 502 CONNECTION_FAILED from quadlets).
	t.Run("UnknownHostIs404NotLeakedAs500", func(t *testing.T) {
		url := suite.ServerURL + "/api/hosts/" + uuid.New().String() + "/containers"
		expectAPIError(t, client, http.MethodGet, url, "",
			http.StatusNotFound, "HOST_NOT_FOUND")
	})
}
