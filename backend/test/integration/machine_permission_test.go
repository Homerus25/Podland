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
	"github.com/stretchr/testify/require"
)

// The level-aware access slice (160-00): machine access is no longer
// boolean. A read-level grantee must keep the read surface (lists,
// gets, logs, generate, events) but lose every mutating route across
// containers, images, and quadlets; write and admin keep mutating.
//
// The fixtures are real in the same sense as the no-access user in
// machine_access_test.go: seeded through the app seam, logged in via
// POST /api/auth/login, and — one step further — granted their level
// through the owner's real POST /api/machines/{id}/access endpoint, so
// the grants live in the same machine_access rows production writes.

// level-user fixtures, built lazily once per test binary (mirrors
// noAccessUserClient). One sync.Once seeds and logs in both users so
// the grant calls happen exactly once even if tests run in any order.
var (
	levelOnce    sync.Once
	levelClients map[string]*http.Client
	levelErr     error
)

// levelUserClient returns an *http.Client authenticated as a user
// granted the given permission level ('read' or 'write') on the shared
// good machine.
func levelUserClient(t *testing.T, permission string) *http.Client {
	t.Helper()
	levelOnce.Do(func() {
		levelClients = make(map[string]*http.Client)
		for _, level := range []string{models.PermissionRead, models.PermissionWrite} {
			username := "it-" + suite.RunID + "-" + level
			password := "it-" + suite.RunID + "-" + level + "-pw-" + uuid.New().String()[:8]
			userID, err := suite.SeedUserWithRole(username, password, models.RoleUser)
			if err != nil {
				levelErr = fmt.Errorf("seed %s user: %w", level, err)
				return
			}
			token, err := realLogin(suite.ServerURL, username, password)
			if err != nil {
				levelErr = fmt.Errorf("login as %s user: %w", level, err)
				return
			}
			// Grant through the real owner endpoint so the level lands
			// in the same machine_access row the middleware reads.
			grantURL := suite.ServerURL + "/api/machines/" + suite.MachineID + "/access"
			grantBody := fmt.Sprintf(`{"userId":%q,"permission":%q}`, userID, level)
			resp, err := suite.AuthenticatedClient().Post(grantURL, "application/json", strings.NewReader(grantBody))
			if err != nil {
				levelErr = fmt.Errorf("grant %s access: %w", level, err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				levelErr = fmt.Errorf("grant %s access: unexpected status %d", level, resp.StatusCode)
				return
			}
			levelClients[level] = &http.Client{
				Timeout: 30 * time.Second,
				Transport: &bearerTransport{
					token: token,
					base:  http.DefaultTransport,
				},
			}
		}
	})
	require.NoError(t, levelErr, "level-user fixtures must be buildable")
	client, ok := levelClients[permission]
	require.Truef(t, ok, "no fixture for permission level %q", permission)
	return client
}

// Test_Integration_MachinePermission_ReadUserRejectedOnMutatingRoutes
// walks every mutating route family gated by RequireMachinePermission:
// a read-level grantee gets 403 ACCESS_DENIED everywhere, with the 403
// firing in middleware before any podman call runs — so nonexistent
// container/image/quadlet IDs are never reached and the shared machine
// is never mutated.
func Test_Integration_MachinePermission_ReadUserRejectedOnMutatingRoutes(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := levelUserClient(t, models.PermissionRead)

	base := suite.ServerURL + "/api/machines/" + suite.MachineID

	t.Run("CreateContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/containers/", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("StartContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/containers/no-such-container/start", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("StopContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/containers/no-such-container/stop", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("DeleteContainerDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, base+"/containers/no-such-container", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("PullImageDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/images/pull", `{"image":"busybox:latest"}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("DeleteImageDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, base+"/images/no-such-image", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("CreateQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("ImportQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/import", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("UpdateQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPut, base+"/quadlets/no-such.container", `{}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("DeleteQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, base+"/quadlets/no-such.container", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("StartQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/no-such.container/start", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("StopQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/no-such.container/stop", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("RestartQuadletDenied", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/no-such.container/restart", "",
			http.StatusForbidden, "ACCESS_DENIED")
	})
	t.Run("CreateSecretDenied", func(t *testing.T) {
		// The 403 fires in middleware before the body is even read, so
		// the (sensitive) value never travels and the shared machine is
		// never mutated.
		expectAPIError(t, client, http.MethodPost, base+"/secrets/", `{"name":"it-denied","value":"nope"}`,
			http.StatusForbidden, "ACCESS_DENIED")
	})
}

// Test_Integration_MachinePermission_ReadUserKeepsReadRoutes pins the
// other half of the read contract: the read surface (lists, gets,
// logs, generate, events) stays open at the subtree level. A 403 here
// would mean the level gate was mounted one level too high.
//
// Quadlet generate is the sharpest probe: it is a POST, so a careless
// wiring could sweep it into the write gate — the handler must be
// reachable at read level, answering with its own domain error for a
// missing source container rather than 403 ACCESS_DENIED.
func Test_Integration_MachinePermission_ReadUserKeepsReadRoutes(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := levelUserClient(t, models.PermissionRead)

	base := suite.ServerURL + "/api/machines/" + suite.MachineID

	for name, url := range map[string]string{
		"GetMachine":     base + "/",
		"ListContainers": base + "/containers/",
		"ListImages":     base + "/images/",
		"ListQuadlets":   base + "/quadlets/",
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := client.Get(url)
			require.NoError(t, err, "GET must reach the in-process server")
			defer resp.Body.Close()
			require.Equalf(t, http.StatusOK, resp.StatusCode,
				"read-level caller must keep read access, got %d", resp.StatusCode)
		})
	}

	t.Run("QuadletGenerateStaysReachable", func(t *testing.T) {
		body := fmt.Sprintf(`{"resourceId":"it-%s-no-such-container","resourceType":"container"}`, suite.RunID)
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/no-such.container/generate", body,
			http.StatusNotFound, "CONTAINER_NOT_FOUND")
	})
}

// Test_Integration_MachinePermission_WriteUserPassesMutatingGates is
// the positive control: a write-level grantee gets PAST the permission
// gate and into the handler, where a nonexistent resource answers with
// the domain's own error — never 403 ACCESS_DENIED. One route per family
// (container delete, image delete, quadlet stop) covers the gate
// wiring; the read/write ordering itself is unit-tested in
// internal/models.
func Test_Integration_MachinePermission_WriteUserPassesMutatingGates(t *testing.T) {
	suite.SkipIfUnavailable(t)
	client := levelUserClient(t, models.PermissionWrite)

	base := suite.ServerURL + "/api/machines/" + suite.MachineID

	t.Run("DeleteContainerReachesHandler", func(t *testing.T) {
		expectAPIError(t, client, http.MethodDelete, base+"/containers/no-such-container", "",
			http.StatusNotFound, "CONTAINER_NOT_FOUND")
	})
	t.Run("DeleteImageReachesHandler", func(t *testing.T) {
		// The image domain currently surfaces an unknown image ID as
		// 500 INTERNAL_ERROR (podman's "image not known" is not mapped
		// to IMAGE_NOT_FOUND on the delete path), so the gate-level
		// contract is asserted instead: anything but 403 ACCESS_DENIED
		// proves the write-level caller reached the handler.
		req, err := http.NewRequest(http.MethodDelete, base+"/images/no-such-image", nil)
		require.NoError(t, err, "DELETE must be buildable")
		resp, err := client.Do(req)
		require.NoError(t, err, "DELETE must reach the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		require.NotEqualf(t, http.StatusForbidden, resp.StatusCode,
			"write-level caller must pass the image-delete gate. body=%s", string(raw))
		var errResp apiErrorResponse
		require.NoError(t, json.Unmarshal(raw, &errResp), "body must decode as structured JSON. body=%s", string(raw))
		require.NotEqual(t, "ACCESS_DENIED", errResp.Error.Code,
			"write-level caller must not be denied on the image-delete gate")
	})
	t.Run("StopQuadletReachesHandler", func(t *testing.T) {
		expectAPIError(t, client, http.MethodPost, base+"/quadlets/no-such.container/stop", "",
			http.StatusNotFound, "QUADLET_NOT_FOUND")
	})
	t.Run("CreateSecretReachesHandler", func(t *testing.T) {
		// A pattern-invalid name proves the write-level caller passed the
		// secret-create gate into the handler (the domain's own 400, not
		// 403) without storing anything on the shared machine — a real
		// create is the secrets slice's own integration test (160-05).
		expectAPIError(t, client, http.MethodPost, base+"/secrets/", `{"name":"-invalid","value":"x"}`,
			http.StatusBadRequest, "VALIDATION_FAILED")
	})
}
