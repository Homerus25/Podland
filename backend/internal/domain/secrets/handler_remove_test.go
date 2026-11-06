package secrets

import (
	"net/http"
	"testing"
	"time"

	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestSecretHandler_BulkRemove_Success pins the bulk-remove contract: the
// body's ids reach the engine, and the answer is 200 with one success
// entry per requested id, in request order.
func TestSecretHandler_BulkRemove_Success(t *testing.T) {
	var gotIDs []string
	engine := &fakeSecretEngine{
		removeFn: func(name string) error {
			gotIDs = append(gotIDs, name)
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "secrets", `{"ids":["db-password","api-key"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "bulk remove answers 200 even when entries fail")
	assert.Equal(t, []string{"db-password", "api-key"}, gotIDs, "every requested id must reach the engine")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2, "one entry per requested id")
	assert.Equal(t, "db-password", got[0].ID)
	assert.True(t, got[0].Success)
	assert.Nil(t, got[0].Error)
	assert.Equal(t, "api-key", got[1].ID)
	assert.True(t, got[1].Success)
}

// TestSecretHandler_BulkRemove_InUseRefusalKeepsBatchGoing pins the refusal
// entry: an in-use secret answers a SECRET_IN_USE entry carrying podman's
// text, and the other ids in the same request still process.
func TestSecretHandler_BulkRemove_InUseRefusalKeepsBatchGoing(t *testing.T) {
	const refusal = "secret db-password is in use by container web"
	engine := &fakeSecretEngine{
		removeFn: func(name string) error {
			if name == "db-password" {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusConflict,
					Message:      refusal,
				}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "secrets", `{"ids":["free-a","db-password","free-b"]}`)

	assert.Equal(t, http.StatusOK, w.Code, "a refused entry must not change the status code")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 3)
	assert.True(t, got[0].Success, "the secret before the refusal must still be removed")
	require.NotNil(t, got[1].Error, "the in-use secret must answer a refused entry")
	assert.False(t, got[1].Success)
	assert.Equal(t, "SECRET_IN_USE", got[1].Error.Code)
	assert.Contains(t, got[1].Error.Message, refusal, "podman's refusal text must reach the client")
	assert.True(t, got[2].Success, "the secret after the refusal must still be removed")
}

// TestSecretHandler_BulkRemove_HasNoForcePath pins that the endpoint's
// contract has no force parameter at all: a body that carries `force`
// anyway is ignored, so an in-use secret still answers a SECRET_IN_USE
// entry — there is no retry that could remove it.
func TestSecretHandler_BulkRemove_HasNoForcePath(t *testing.T) {
	engine := &fakeSecretEngine{
		removeFn: func(name string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "secret is in use"}
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "secrets", `{"ids":["db-password"],"force":true}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Error, "force is not part of this endpoint's contract, so it cannot rescue an in-use secret")
	assert.False(t, got[0].Success)
	assert.Equal(t, "SECRET_IN_USE", got[0].Error.Code)
}

// TestSecretHandler_BulkRemove_MissingSecretIsNotFoundEntry pins the
// vanished secret branch: a missing id answers a SECRET_NOT_FOUND entry and
// the rest of the batch still processes.
func TestSecretHandler_BulkRemove_MissingSecretIsNotFoundEntry(t *testing.T) {
	engine := &fakeSecretEngine{
		removeFn: func(name string) error {
			if name == "ghost" {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such secret"}
			}
			return nil
		},
	}

	w := testutils.BulkRemoveRequest(newTestHandler(engine).BulkRemove, "secrets", `{"ids":["ghost","real"]}`)

	assert.Equal(t, http.StatusOK, w.Code)
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Error)
	assert.False(t, got[0].Success)
	assert.Equal(t, "SECRET_NOT_FOUND", got[0].Error.Code)
	assert.True(t, got[1].Success, "a missing id must not abort the batch")
}

// --- Route wiring: the secrets subtree gates bulk remove at write level ---

func bulkRemoveAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, handler *SecretHandler) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	// The literal /remove route must beat the /{secretId} wildcard or
	// remove calls get swallowed as a secret named "remove".
	r.Handle("POST /api/hosts/{id}/secrets/remove", hostAccess(requireWrite(http.HandlerFunc(handler.BulkRemove))))
	r.Handle("GET /api/hosts/{id}/secrets/{secretId}", hostAccess(http.HandlerFunc(handler.Get)))
	return r
}

// TestSecretHandler_BulkRemove_WriteGate pins the write gate through the
// real middleware: a read-level grantee is denied with 403 before the
// engine is ever reached, while write- and admin-level grantees pass.
func TestSecretHandler_BulkRemove_WriteGate(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		expectStatus int
		expectCalled bool
	}{
		{name: "read-level grantee is denied", level: models.PermissionRead, expectStatus: http.StatusForbidden, expectCalled: false},
		{name: "write-level grantee passes", level: models.PermissionWrite, expectStatus: http.StatusOK, expectCalled: true},
		{name: "admin-level grantee passes", level: models.PermissionAdmin, expectStatus: http.StatusOK, expectCalled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostRepo := testutils.NewMockHostRepository()
			hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockHostAccessRepository()
			accessRepo.AddAccess("host-1", "grantee-1", tt.level, time.Now())
			called := false
			engine := &fakeSecretEngine{
				removeFn: func(string) error {
					called = true
					return nil
				},
			}
			router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

			w := testutils.BulkRemoveRequestAsUser(router, "secrets", "grantee-1", `{"ids":["db-password"]}`)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "the engine must run=%v for level %s — the gate fires before any podman call", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}

// TestSecretHandler_BulkRemove_LiteralRouteWins pins that the literal
// /remove route beats the /{secretId} subtree: with both mounted the way
// app.go mounts them, POST /secrets/remove reaches the bulk handler (a
// swallowed route would answer 404/405, not a report).
func TestSecretHandler_BulkRemove_LiteralRouteWins(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "grantee-1", models.PermissionWrite, time.Now())
	engine := &fakeSecretEngine{
		removeFn: func(string) error { return nil },
	}
	router := bulkRemoveAccessRouter(hostRepo, accessRepo, newTestHandler(engine))

	w := testutils.BulkRemoveRequestAsUser(router, "secrets", "grantee-1", `{"ids":["db-password"]}`)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "the literal /remove route must win over the {secretId} subtree")
	got := testutils.DecodeBulkRemoveReport(t, w)
	require.Len(t, got, 1)
	assert.Equal(t, "db-password", got[0].ID)
}
