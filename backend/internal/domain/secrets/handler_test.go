package secrets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// adminRepo answers every permission lookup with admin, so reveal tests
// pass the service's admin gate (the deny path is pinned in the service
// tests).
type adminRepo struct{}

func (adminRepo) GetPermission(hostID, userID string) (string, error) {
	return models.PermissionAdmin, nil
}

// newTestHandler builds the handler around the real service running over
// the given fake engine; tests arm only the operations their scenario
// needs (an unarmed operation panics if the handler reaches it). The
// access repo always answers admin; the below-admin reveal refusal is a
// service-level ruling pinned in the service tests.
func newTestHandler(engine *fakeSecretEngine) *SecretHandler {
	return NewSecretHandler(NewSecretService(fakeEngineProvider{engine: engine}, adminRepo{}))
}

// secretFixture builds a SecretInfoReport like podman's list/inspect
// answer: metadata under Spec, optionally carrying a value.
func secretFixture(id, name string, createdAt time.Time, secretData string) *entities.SecretInfoReport {
	return &entities.SecretInfoReport{
		ID:         id,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
		Spec:       entities.SecretSpec{Name: name, Driver: entities.SecretDriverSpec{Name: "file"}, Labels: map[string]string{"app": "web"}},
		SecretData: secretData,
	}
}

func TestSecretHandler_List(t *testing.T) {
	tests := []struct {
		name           string
		reports        []*entities.SecretInfoReport
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful list",
			reports: []*entities.SecretInfoReport{
				secretFixture("3f2e1d0c", "db-password", mustParseSecretTime("2026-01-02T15:04:05Z"), ""),
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"3f2e1d0c","name":"db-password","driver":"file","createdAt":"2026-01-02T15:04:05Z","updatedAt":"2026-01-02T15:04:05Z","labels":{"app":"web"}}]`,
		},
		{
			name: "successful list without labels omits the field",
			reports: []*entities.SecretInfoReport{
				{ID: "9a8b7c6d", CreatedAt: mustParseSecretTime("2026-02-04T10:11:12Z"), UpdatedAt: mustParseSecretTime("2026-02-04T10:11:12Z"),
					Spec: entities.SecretSpec{Name: "api-token", Driver: entities.SecretDriverSpec{Name: "file"}}},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"9a8b7c6d","name":"api-token","driver":"file","createdAt":"2026-02-04T10:11:12Z","updatedAt":"2026-02-04T10:11:12Z"}]`,
		},
		{
			name:           "successful list empty",
			reports:        nil,
			expectedStatus: http.StatusOK,
			expectedBody:   `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeSecretEngine{
				listFn: func() ([]*entities.SecretInfoReport, error) {
					return tt.reports, nil
				},
			}

			rr := httptest.NewRecorder()
			newTestHandler(engine).List(rr, listRequest())

			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.JSONEq(t, tt.expectedBody, rr.Body.String())
		})
	}
}

func mustParseSecretTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic("bad fixture timestamp: " + err.Error())
	}
	return parsed
}

func listRequest() *http.Request {
	req := httptest.NewRequest("GET", "/api/hosts/host123/secrets", nil)
	ctx := context.WithValue(req.Context(), contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)
	req.SetPathValue("id", "host123")
	return req
}

// --- Inspect (GET /api/hosts/{id}/secrets/{secretId}) ---

// TestSecretHandler_Get_MetadataOnly pins the happy path of a metadata
// inspect: 200 with the metadata JSON and NO secretData field — even when
// the report carried a value, mapping must not have copied it.
func TestSecretHandler_Get_MetadataOnly(t *testing.T) {
	createdAt := mustParseSecretTime("2026-01-02T15:04:05Z")
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			assert.False(t, reveal, "a metadata inspect must not ask for the value")
			return secretFixture("3f2e1d0c", "db-password", createdAt, "must-not-leak"), nil
		},
	}

	rr := httptest.NewRecorder()
	newTestHandler(engine).Get(rr, getRequest("/api/hosts/host123/secrets/3f2e1d0c"))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"id": "3f2e1d0c",
		"name": "db-password",
		"driver": "file",
		"createdAt": "2026-01-02T15:04:05Z",
		"updatedAt": "2026-01-02T15:04:05Z",
		"labels": {"app": "web"}
	}`, rr.Body.String(), "a metadata inspect must answer WITHOUT a secretData field")
}

// TestSecretHandler_Get_RevealReturnsValue pins the reveal query contract:
// a successful ?reveal=true's JSON carries the secretData field next to
// the metadata.
func TestSecretHandler_Get_RevealReturnsValue(t *testing.T) {
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			assert.True(t, reveal, "?reveal=true must reach the host lookup as a reveal")
			return secretFixture("3f2e1d0c", "db-password", mustParseSecretTime("2026-01-02T15:04:05Z"), "hunter2"), nil
		},
	}

	rr := httptest.NewRecorder()
	newTestHandler(engine).Get(rr, getRequest("/api/hosts/host123/secrets/db-password?reveal=true"))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"id": "3f2e1d0c",
		"name": "db-password",
		"driver": "file",
		"createdAt": "2026-01-02T15:04:05Z",
		"updatedAt": "2026-01-02T15:04:05Z",
		"labels": {"app": "web"},
		"secretData": "hunter2"
	}`, rr.Body.String(), "a reveal must answer with the secretData field")
}

// TestSecretHandler_Get_RevealParamDecoding pins the reveal query-param
// decoding: only the exact value "true" asks for the value (?reveal=false
// and a missing param are both a metadata inspect).
func TestSecretHandler_Get_RevealParamDecoding(t *testing.T) {
	var gotReveals []bool
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			gotReveals = append(gotReveals, reveal)
			return secretFixture("3f2e1d0c", "db-password", time.Time{}, ""), nil
		},
	}
	handler := newTestHandler(engine)

	for _, target := range []string{
		"/api/hosts/host123/secrets/db-password?reveal=true",
		"/api/hosts/host123/secrets/db-password?reveal=false",
		"/api/hosts/host123/secrets/3f2e1d0c",
	} {
		rr := httptest.NewRecorder()
		handler.Get(rr, getRequest(target))
		require.Equal(t, http.StatusOK, rr.Code, "target %s must succeed", target)
	}

	assert.Equal(t, []bool{true, false, false}, gotReveals,
		"only ?reveal=true may reach the host as a reveal request")
}

func getRequest(target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	ctx := context.WithValue(req.Context(), contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)
	req.SetPathValue("id", "host123")
	req.SetPathValue("secretId", "db-password")
	return req
}

// TestSecretHandler_Get_ReadLevelGranteeReachesInspectRoute pins the
// route mounting: GET /api/hosts/{id}/secrets/{secretId} rides the
// RequireHostAccess subtree, so a read-level grantee reaches the
// inspect handler (metadata is visible at read; the reveal gate is the
// service's, not the route's).
func TestSecretHandler_Get_ReadLevelGranteeReachesInspectRoute(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return secretFixture("3f2e1d0c", "db-password", time.Time{}, ""), nil
		},
	}
	router := secretsAccessRouter(hostRepo, accessRepo, newTestHandler(engine).service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/secrets/db-password", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the inspect endpoint")
	assert.NotContains(t, w.Body.String(), "secretData",
		"the metadata answer must not carry a secretData field")
}

// --- Create (POST /api/hosts/{id}/secrets) ---

// newCreateRequest builds a POST secrets request with the given JSON body,
// carrying the given user ID and host ID the way the mounted route
// would extract them.
func newCreateRequest(userID, hostID, body string) *http.Request {
	req := httptest.NewRequest("POST", "/api/hosts/"+hostID+"/secrets", strings.NewReader(body))
	ctx := req.Context()
	if userID != "" {
		ctx = context.WithValue(ctx, contextutils.UserIDContextKey, userID)
	}
	req = req.WithContext(ctx)
	req.SetPathValue("id", hostID)
	return req
}

// TestSecretHandler_Create_HappyPath pins the create contract through the
// real service: 201 with only {id} in the body, the name forwarded
// TRIMMED, and the value forwarded byte-identical (significant whitespace
// intact) into the host's create call.
func TestSecretHandler_Create_HappyPath(t *testing.T) {
	var gotName, gotValue string
	engine := &fakeSecretEngine{
		createFn: func(name, value string) (*entities.SecretCreateReport, error) {
			gotName, gotValue = name, value
			return &entities.SecretCreateReport{ID: "3f2e1d0c"}, nil
		},
	}

	rr := httptest.NewRecorder()
	newTestHandler(engine).Create(rr, newCreateRequest("user123", "host123",
		`{"name":"  db-password  ","value":"  hunter2 \nnext line\n"}`))

	assert.Equal(t, http.StatusCreated, rr.Code, "a successful create must answer 201")
	assert.JSONEq(t, `{"id":"3f2e1d0c"}`, rr.Body.String(),
		"the create response must carry the created ID only — never the value")
	assert.Equal(t, "db-password", gotName, "the trimmed name must reach the host")
	assert.Equal(t, "  hunter2 \nnext line\n", gotValue,
		"the value must reach the host byte-identical — never trimmed")
}

// TestSecretHandler_Create_MaxSizeValueStillPasses pins the interplay of
// the two caps: a value of exactly maxSecretValueBytes (1 MiB) is the
// largest legitimate secret create body, so it must clear both the
// service's value cap and web.Decode's global 10 MiB body cap — 201,
// value byte-identical.
func TestSecretHandler_Create_MaxSizeValueStillPasses(t *testing.T) {
	var gotValue string
	engine := &fakeSecretEngine{
		createFn: func(name, value string) (*entities.SecretCreateReport, error) {
			gotValue = value
			return &entities.SecretCreateReport{ID: "3f2e1d0c"}, nil
		},
	}

	maxValue := strings.Repeat("v", 1048576)
	rr := httptest.NewRecorder()
	newTestHandler(engine).Create(rr, newCreateRequest("user123", "host123",
		`{"name":"db-password","value":"`+maxValue+`"}`))

	assert.Equal(t, http.StatusCreated, rr.Code,
		"a 1 MiB value is the largest legitimate create and must pass the global body cap")
	assert.Equal(t, maxValue, gotValue,
		"the max-size value must reach the host byte-identical")
}

// TestSecretHandler_Create_ValidationFailures pins the 400 contract: a
// missing/oversized/pattern-breaking name or an empty/oversized value is
// refused with VALIDATION_FAILED before the host is asked (body
// decoding itself is pinned once by web.Decode). The engine stays
// unarmed: any create reaching it panics the test.
func TestSecretHandler_Create_ValidationFailures(t *testing.T) {
	oversizedName := strings.Repeat("a", 256)
	oversizedValue := strings.Repeat("v", 1048576+1)
	tests := []struct {
		name       string
		body       string
		expectCode string
	}{
		{name: "missing name", body: `{"value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "whitespace-only name", body: `{"name":"   ","value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "oversized name", body: `{"name":"` + oversizedName + `","value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "name with leading dash", body: `{"name":"-db-password","value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "name with space", body: `{"name":"db password","value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "name with slash", body: `{"name":"db/password","value":"hunter2"}`, expectCode: "VALIDATION_FAILED"},
		{name: "missing value", body: `{"name":"db-password"}`, expectCode: "VALIDATION_FAILED"},
		{name: "empty value", body: `{"name":"db-password","value":""}`, expectCode: "VALIDATION_FAILED"},
		{name: "oversized value", body: `{"name":"db-password","value":"` + oversizedValue + `"}`, expectCode: "VALIDATION_FAILED"},
		// The cap is BYTES: a rune count under the limit must still be
		// refused when its UTF-8 encoding exceeds it (validator's
		// min/max would count runes and wave this through).
		{
			name:       "multi-byte value under the rune cap but over the byte cap",
			body:       `{"name":"db-password","value":"` + strings.Repeat("€", 600000) + `"}`,
			expectCode: "VALIDATION_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeSecretEngine{}

			rr := httptest.NewRecorder()
			newTestHandler(engine).Create(rr, newCreateRequest("user123", "host123", tt.body))

			assert.Equal(t, http.StatusBadRequest, rr.Code, "invalid input must be refused with 400")
			assert.Contains(t, rr.Body.String(), tt.expectCode, "error body should carry the %s code", tt.expectCode)
		})
	}
}

// TestSecretHandler_Create_RejectsValueWithoutEchoingIt pins the redaction
// discipline at the validation boundary: a rejected request's response (and
// thus nothing the handler could log from it) must not echo the submitted
// value — only the failing field is named.
func TestSecretHandler_Create_RejectsValueWithoutEchoingIt(t *testing.T) {
	engine := &fakeSecretEngine{}

	rr := httptest.NewRecorder()
	newTestHandler(engine).Create(rr, newCreateRequest("user123", "host123",
		`{"name":"-bad-name","value":"top-secret-payload"}`))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.NotContains(t, rr.Body.String(), "top-secret-payload",
		"a validation failure must never echo the submitted value")
}

// TestSecretHandler_Create_WriteGate pins the route mounting for the
// create route: POST /secrets is gated at write level, so a read-level
// grantee (who keeps metadata visibility) is refused with 403 in
// middleware — the host is never asked to store anything — while
// write- and admin-level grantees reach the handler.
func TestSecretHandler_Create_WriteGate(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		expectStatus int
		expectCalled bool
	}{
		{name: "read-level grantee is denied", level: models.PermissionRead, expectStatus: http.StatusForbidden, expectCalled: false},
		{name: "write-level grantee passes", level: models.PermissionWrite, expectStatus: http.StatusCreated, expectCalled: true},
		{name: "admin-level grantee passes", level: models.PermissionAdmin, expectStatus: http.StatusCreated, expectCalled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostRepo := testutils.NewMockHostRepository()
			hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockHostAccessRepository()
			accessRepo.AddAccess("host-1", "grantee-1", tt.level, time.Now())
			called := false
			engine := &fakeSecretEngine{
				createFn: func(name, value string) (*entities.SecretCreateReport, error) {
					called = true
					return &entities.SecretCreateReport{ID: "3f2e1d0c"}, nil
				},
			}
			router := secretsAccessRouter(hostRepo, accessRepo, newTestHandler(engine).service)

			req := httptest.NewRequest("POST", "/api/hosts/host-1/secrets",
				strings.NewReader(`{"name":"db-password","value":"hunter2"}`))
			req = req.WithContext(contextutils.SetUserID(req.Context(), "grantee-1"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode,
				"level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called,
				"handler must run=%v for level %s", tt.expectCalled, tt.level)
			if tt.expectStatus == http.StatusForbidden {
				assert.Contains(t, w.Body.String(), "ACCESS_DENIED",
					"the deny must carry the ACCESS_DENIED code")
			}
		})
	}
}

// --- Route wiring: the RequireHostAccess subtree is the read gate ---
//
// These tests mount the secrets route on a chi subtree exactly the way
// internal/app/app.go does (RequireHostAccess at the /hosts/{id}
// subtree level, r.Get("/secrets", ...)) so the access semantics the
// contract promises are exercised through the real URL-param extraction:
// unknown host → 404 HOST_NOT_FOUND, known host without an
// access row → 403 ACCESS_DENIED, and a read-level grantee through to the
// handler (metadata is visible at read).

// secretsAccessRouter mirrors app.go's mounting of the secrets routes.
func secretsAccessRouter(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, service *SecretService) http.Handler {
	hostAccess := appmiddleware.RequireHostAccess(hostRepo, accessRepo)
	requireWrite := appmiddleware.RequireHostPermission(hostRepo, accessRepo, models.PermissionWrite)
	r := http.NewServeMux()
	r.Handle("GET /api/hosts/{id}/secrets", hostAccess(http.HandlerFunc(NewSecretHandler(service).List)))
	r.Handle("POST /api/hosts/{id}/secrets", hostAccess(requireWrite(http.HandlerFunc(NewSecretHandler(service).Create))))
	r.Handle("GET /api/hosts/{id}/secrets/{secretId}", hostAccess(http.HandlerFunc(NewSecretHandler(service).Get)))
	r.Handle("GET /api/hosts/{id}/secrets/{secretId}/inspect", hostAccess(http.HandlerFunc(NewSecretHandler(service).Inspect)))
	r.Handle("DELETE /api/hosts/{id}/secrets/{secretId}", hostAccess(requireWrite(http.HandlerFunc(NewSecretHandler(service).Delete))))
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

func TestSecretHandler_List_UnknownHostIsNotFound(t *testing.T) {
	engine := &fakeSecretEngine{
		listFn: func() ([]*entities.SecretInfoReport, error) {
			return nil, nil
		},
	}
	router := secretsAccessRouter(testutils.NewMockHostRepository(), testutils.NewMockHostAccessRepository(), newTestHandler(engine).service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/no-such-host/secrets", "owner-1"))

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "unknown host must be rejected with 404")
	assert.Contains(t, w.Body.String(), "HOST_NOT_FOUND", "error body should carry the HOST_NOT_FOUND code")
}

func TestSecretHandler_List_UserWithoutAccessIsForbidden(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	engine := &fakeSecretEngine{
		listFn: func() ([]*entities.SecretInfoReport, error) {
			return nil, nil
		},
	}
	router := secretsAccessRouter(hostRepo, testutils.NewMockHostAccessRepository(), newTestHandler(engine).service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/secrets", "stranger-1"))

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "caller without an access row must be rejected with 403")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "error body should carry the ACCESS_DENIED code")
}

func TestSecretHandler_List_ReadLevelGranteeReachesHandler(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockHostAccessRepository()
	accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
	engine := &fakeSecretEngine{
		listFn: func() ([]*entities.SecretInfoReport, error) {
			return nil, nil
		},
	}
	router := secretsAccessRouter(hostRepo, accessRepo, newTestHandler(engine).service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/hosts/host-1/secrets", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "secret metadata is visible at read level")
}

// TestSecretHandler_Delete_Success pins the delete contract: a successful
// removal answers 204 with an empty body (the frontend series (170)
// invalidates the list on the 204 — there is nothing to parse).
func TestSecretHandler_Delete_Success(t *testing.T) {
	var gotID string
	engine := &fakeSecretEngine{
		removeFn: func(name string) error {
			gotID = name
			return nil
		},
	}

	req := httptest.NewRequest("DELETE", "/api/hosts/host123/secrets/db-password", nil)
	ctx := context.WithValue(req.Context(), contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)
	req.SetPathValue("id", "host123")
	req.SetPathValue("secretId", "db-password")

	rr := httptest.NewRecorder()
	newTestHandler(engine).Delete(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String(), "a 204 must carry no body")
	assert.Equal(t, "db-password", gotID, "the URL param must be the removed secret")
}

// TestSecretHandler_Delete_RouteLevelGating pins the mounted-route contract
// for delete (the DELETE route rides RequireHostPermission(write), so a
// read-level grantee is refused 403 before the handler runs, a write-level
// grantee goes through, and an unknown host is 404'd by
// RequireHostAccess).
func TestSecretHandler_Delete_RouteLevelGating(t *testing.T) {
	newRouter := func() (http.Handler, *int) {
		calls := 0
		engine := &fakeSecretEngine{
			removeFn: func(name string) error {
				calls++
				return nil
			},
		}
		hostRepo := testutils.NewMockHostRepository()
		hostRepo.AddHost(&models.Host{ID: "host-1", Name: "host-one", OwnerID: "owner-1"})
		accessRepo := testutils.NewMockHostAccessRepository()
		accessRepo.AddAccess("host-1", "reader-1", models.PermissionRead, time.Now())
		accessRepo.AddAccess("host-1", "writer-1", models.PermissionWrite, time.Now())
		return secretsAccessRouter(hostRepo, accessRepo, newTestHandler(engine).service), &calls
	}

	t.Run("read-level grantee is forbidden", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/hosts/host-1/secrets/db-password", "reader-1"))
		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "a read-level grantee must be refused")
		assert.Zero(t, *calls, "a refused delete must never reach the handler")
	})

	t.Run("write-level grantee goes through", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/hosts/host-1/secrets/db-password", "writer-1"))
		assert.Equal(t, http.StatusNoContent, w.Result().StatusCode, "a write-level grantee must reach the 204")
		assert.Equal(t, 1, *calls, "the handler must run once for a write-level grantee")
	})

	t.Run("unknown host is not found", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/hosts/no-such-host/secrets/db-password", "writer-1"))
		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "an unknown host must be refused before the handler")
		assert.Zero(t, *calls, "a 404 host must never reach the handler")
	})
}
