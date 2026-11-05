package secrets

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// --- Inspect (GET /api/hosts/{id}/secrets/{secretId}/inspect) ---

// inspectRequest serves GET
// /api/hosts/{id}/secrets/{secretId}/inspect through the handler with
// an authenticated test identity, the way the router would.
func inspectRequest(handler http.HandlerFunc, secretID string) *httptest.ResponseRecorder {
	return testutils.Perform(handler, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/secrets/" + secretID + "/inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host", "secretId": secretID},
	})
}

// TestSecretHandler_Inspect_ReturnsRawReport pins the raw-inspect contract:
// 200 with the secret's raw podman inspect report as the top-level body —
// the entity's own field names (ID, Spec, ...), NOT the Get endpoint's DTO
// shape — and no secretData field.
func TestSecretHandler_Inspect_ReturnsRawReport(t *testing.T) {
	var gotName string
	var gotReveal bool
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			gotName, gotReveal = name, reveal
			return secretFixture("3f2e1d0c", "db-password", mustParseSecretTime("2026-01-02T15:04:05Z"), ""), nil
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "db-password")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "db-password", gotName, "the URL param must be the inspected secret")
	assert.False(t, gotReveal, "the raw inspect must never ask the engine for the value")
	assert.JSONEq(t, `{
		"ID": "3f2e1d0c",
		"CreatedAt": "2026-01-02T15:04:05Z",
		"UpdatedAt": "2026-01-02T15:04:05Z",
		"Spec": {
			"Name": "db-password",
			"Driver": {"Name": "file", "Options": null},
			"Labels": {"app": "web"}
		}
	}`, w.Body.String(), "the body must be the raw inspect report, not the Get DTO")
	assert.NotContains(t, w.Body.String(), "SecretData",
		"the raw inspect answer must not carry a SecretData field")
	assert.NotContains(t, w.Body.String(), "secretData",
		"the raw inspect answer must not carry a secretData field")
}

// TestSecretHandler_Inspect_NotFound pins the 404 envelope: a missing
// secret answers the domain's SECRET_NOT_FOUND error body.
func TestSecretHandler_Inspect_NotFound(t *testing.T) {
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusNotFound,
				Message:      "no such secret with name or id \"nope\"",
				Because:      "no such secret",
			}
		},
	}

	w := inspectRequest(newTestHandler(engine).Inspect, "nope")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "SECRET_NOT_FOUND", "error body should carry the SECRET_NOT_FOUND code")
}

// TestSecretHandler_Inspect_MissingSecretID pins the URL-param guard: a
// request without the {secretId} param answers 400 MISSING_SECRET_ID before
// the service runs (the engine stays unarmed — any call panics the test).
func TestSecretHandler_Inspect_MissingSecretID(t *testing.T) {
	engine := &fakeSecretEngine{}

	w := testutils.Perform(newTestHandler(engine).Inspect, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/test-host/secrets//inspect",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "MISSING_SECRET_ID", "error body should carry the MISSING_SECRET_ID code")
}

// TestSecretHandler_Inspect_ReadLevelGranteeReachesRoute pins the route
// mounting: GET /api/hosts/{id}/secrets/{secretId}/inspect rides the
// RequireHostAccess subtree at read level (no requireWrite), so a
// read-level grantee reaches the handler — metadata is visible at read —
// and the answer carries no value.
func TestSecretHandler_Inspect_ReadLevelGranteeReachesRoute(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			assert.False(t, reveal, "the raw inspect must never ask the engine for the value")
			return secretFixture("3f2e1d0c", "db-password", time.Time{}, ""), nil
		},
	}
	router := secretsAccessRouter(hostRepo, accessRepo, newTestHandler(engine).service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/secrets/db-password/inspect", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the raw inspect endpoint")
	assert.NotContains(t, w.Body.String(), "SecretData",
		"the raw inspect answer must not carry the secret value")
}
