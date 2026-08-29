package secrets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errGeneric is a non-AppError used to exercise the handler's generic
// error→500 mapping. The app errors package has no New constructor, so we
// synthesize a plain error here (same idiom as the quadlets handler tests).
var errGeneric = fmt.Errorf("boom")

func TestSecretHandler_List(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		mockSecrets    []SecretListResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:      "successful list",
			userID:    "user123",
			machineID: "machine123",
			mockSecrets: []SecretListResponse{
				{
					ID:        "3f2e1d0c",
					Name:      "db-password",
					Driver:    "file",
					CreatedAt: "2026-01-02T15:04:05Z",
					UpdatedAt: "2026-01-03T16:05:06Z",
					Labels:    map[string]string{"app": "web"},
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"3f2e1d0c","name":"db-password","driver":"file","createdAt":"2026-01-02T15:04:05Z","updatedAt":"2026-01-03T16:05:06Z","labels":{"app":"web"}}]` + "\n",
		},
		{
			name:           "successful list without labels omits the field",
			userID:         "user123",
			machineID:      "machine123",
			mockSecrets:    []SecretListResponse{{ID: "9a8b7c6d", Name: "api-token", Driver: "file", CreatedAt: "2026-02-04T10:11:12Z", UpdatedAt: "2026-02-04T10:11:12Z"}},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"9a8b7c6d","name":"api-token","driver":"file","createdAt":"2026-02-04T10:11:12Z","updatedAt":"2026-02-04T10:11:12Z"}]` + "\n",
		},
		{
			name:           "successful list empty",
			userID:         "user123",
			machineID:      "machine123",
			mockSecrets:    []SecretListResponse{},
			expectedStatus: http.StatusOK,
			expectedBody:   `[]` + "\n",
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "",
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "",
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:           "non-app error falls back to internal error",
			userID:         "user123",
			machineID:      "machine123",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockSecretService{
				mockList: func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockSecrets, nil
				},
			}

			handler := NewSecretHandler(mockService)

			req := httptest.NewRequest("GET", "/api/machines/"+tt.machineID+"/secrets", nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.List(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				actualBody := rr.Body.String()
				if actualBody != tt.expectedBody {
					t.Errorf("handler returned unexpected body: got %q want %q", actualBody, tt.expectedBody)
				}
			}
		})
	}
}

type MockSecretService struct {
	mockList   func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error)
	mockGet    func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error)
	mockCreate func(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error)
	mockDelete func(ctx context.Context, userID, machineID, secretID string) error
}

func (m *MockSecretService) List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
	if m.mockList != nil {
		return m.mockList(ctx, userID, machineID)
	}
	return nil, nil
}

func (m *MockSecretService) Get(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
	if m.mockGet != nil {
		return m.mockGet(ctx, userID, machineID, secretID, reveal)
	}
	return nil, nil
}

func (m *MockSecretService) Create(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
	if m.mockCreate != nil {
		return m.mockCreate(ctx, userID, machineID, req)
	}
	return nil, nil
}

func (m *MockSecretService) Delete(ctx context.Context, userID, machineID, secretID string) error {
	if m.mockDelete != nil {
		return m.mockDelete(ctx, userID, machineID, secretID)
	}
	return nil
}

// TestSecretHandler_List_ForwardsCallerAndMachine pins the handler→service
// seam: the authenticated user ID and the {id} URL param must reach
// Service.List unchanged (the connection manager resolves the tunnel by
// exactly this pair).
func TestSecretHandler_List_ForwardsCallerAndMachine(t *testing.T) {
	var gotUser, gotMachine string
	mockService := &MockSecretService{
		mockList: func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
			gotUser, gotMachine = userID, machineID
			return []SecretListResponse{}, nil
		},
	}

	handler := NewSecretHandler(mockService)

	req := httptest.NewRequest("GET", "/api/machines/machine123/secrets", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.List(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "user123", gotUser, "handler must forward the authenticated user ID")
	assert.Equal(t, "machine123", gotMachine, "handler must forward the machine ID URL param")
}

// --- Inspect (GET /api/machines/{id}/secrets/{secretId}) ---

// TestSecretHandler_Get_MetadataOnly pins the happy path of a metadata
// inspect: 200 with the metadata JSON and NO secretData field, exactly as
// the omitempty-tagged DTO renders it.
func TestSecretHandler_Get_MetadataOnly(t *testing.T) {
	mockService := &MockSecretService{
		mockGet: func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
			return &SecretInspectResponse{
				SecretListResponse: SecretListResponse{
					ID:        "3f2e1d0c",
					Name:      "db-password",
					Driver:    "file",
					CreatedAt: "2026-01-02T15:04:05Z",
					UpdatedAt: "2026-01-03T16:05:06Z",
				},
			}, nil
		},
	}
	handler := NewSecretHandler(mockService)

	req := httptest.NewRequest("GET", "/api/machines/machine123/secrets/3f2e1d0c", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	rctx.URLParams.Add("secretId", "3f2e1d0c")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.Get(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"id": "3f2e1d0c",
		"name": "db-password",
		"driver": "file",
		"createdAt": "2026-01-02T15:04:05Z",
		"updatedAt": "2026-01-03T16:05:06Z"
	}`, rr.Body.String(), "a metadata inspect must answer WITHOUT a secretData field")
}

// TestSecretHandler_Get_RevealReturnsValue pins that a successful reveal's
// JSON carries the secretData field next to the metadata.
func TestSecretHandler_Get_RevealReturnsValue(t *testing.T) {
	mockService := &MockSecretService{
		mockGet: func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
			return &SecretInspectResponse{
				SecretListResponse: SecretListResponse{ID: "3f2e1d0c", Name: "db-password", Driver: "file"},
				SecretData:         "hunter2",
			}, nil
		},
	}
	handler := NewSecretHandler(mockService)

	req := httptest.NewRequest("GET", "/api/machines/machine123/secrets/db-password?reveal=true", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	rctx.URLParams.Add("secretId", "db-password")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.Get(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"id": "3f2e1d0c",
		"name": "db-password",
		"driver": "file",
		"createdAt": "",
		"updatedAt": "",
		"secretData": "hunter2"
	}`, rr.Body.String(), "a reveal must answer with the secretData field")
}

// TestSecretHandler_Get_ForwardsArgs pins the handler→service seam: the
// authenticated user ID, the {id} and {secretId} URL params, and the
// reveal flag parsed from ?reveal= must reach Service.Get unchanged
// (?reveal=false and a missing param are both false).
func TestSecretHandler_Get_ForwardsArgs(t *testing.T) {
	calls := []struct {
		userID   string
		machine  string
		secretID string
		reveal   bool
	}{}
	mockService := &MockSecretService{
		mockGet: func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
			calls = append(calls, struct {
				userID   string
				machine  string
				secretID string
				reveal   bool
			}{userID, machineID, secretID, reveal})
			return &SecretInspectResponse{}, nil
		},
	}
	handler := NewSecretHandler(mockService)

	for _, target := range []string{
		"/api/machines/machine123/secrets/db-password?reveal=true",
		"/api/machines/machine123/secrets/db-password?reveal=false",
		"/api/machines/machine123/secrets/3f2e1d0c",
	} {
		req := httptest.NewRequest("GET", target, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "machine123")
		rctx.URLParams.Add("secretId", "db-password")
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler.Get(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, "target %s must succeed", target)
	}

	require.Len(t, calls, 3, "every request must reach the service")
	for _, call := range calls {
		assert.Equal(t, "user123", call.userID, "handler must forward the authenticated user ID")
		assert.Equal(t, "machine123", call.machine, "handler must forward the machine ID URL param")
		assert.Equal(t, "db-password", call.secretID, "handler must forward the secret ID URL param")
	}
	assert.True(t, calls[0].reveal, "?reveal=true must reach the service as true")
	assert.False(t, calls[1].reveal, "?reveal=false must reach the service as false")
	assert.False(t, calls[2].reveal, "a missing reveal param must reach the service as false")
}

// TestSecretHandler_Get_ErrorMapping pins the error pass-throughs: the
// service's AppErrors keep their HTTP status (403 ACCESS_DENIED for a
// below-admin reveal, 404 SECRET_NOT_FOUND for an unknown secret, 502
// CONNECTION_FAILED for a dead tunnel) and a non-AppError falls back to
// 500.
func TestSecretHandler_Get_ErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "below-admin reveal is access denied",
			mockError:      errors.ErrAccessDenied,
			expectedStatus: http.StatusForbidden,
			expectedBody:   "ACCESS_DENIED",
		},
		{
			name:           "unknown secret is not found",
			mockError:      errors.ErrSecretNotFound,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "SECRET_NOT_FOUND",
		},
		{
			// 5xx bodies are rewritten generic by RespondWithError
			// (handlers.go), so the 502 is asserted via status alone.
			name:           "connection failed",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:           "non-app error falls back to internal error",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockSecretService{
				mockGet: func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
					return nil, tt.mockError
				},
			}
			handler := NewSecretHandler(mockService)

			req := httptest.NewRequest("GET", "/api/machines/machine123/secrets/db-password", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "machine123")
			rctx.URLParams.Add("secretId", "db-password")
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Get(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.expectedBody)
		})
	}
}

// TestSecretHandler_Get_MissingContextAndParams pins the request-shape
// guards: no authenticated user → 401, no {secretId} URL param → 400.
func TestSecretHandler_Get_MissingContextAndParams(t *testing.T) {
	newReq := func(userID, secretID string) *http.Request {
		req := httptest.NewRequest("GET", "/api/machines/machine123/secrets/"+secretID, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "machine123")
		if secretID != "" {
			rctx.URLParams.Add("secretId", secretID)
		}
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		if userID != "" {
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, userID)
		}
		return req.WithContext(ctx)
	}

	t.Run("missing user ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		NewSecretHandler(&MockSecretService{}).Get(rr, newReq("", "db-password"))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("missing secret ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		NewSecretHandler(&MockSecretService{}).Get(rr, newReq("user123", ""))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestSecretHandler_Get_ReadLevelGranteeReachesInspectRoute pins the
// route mounting: GET /api/machines/{id}/secrets/{secretId} rides the
// RequireMachineAccess subtree, so a read-level grantee reaches the
// inspect handler (metadata is visible at read; the reveal gate is the
// service's, not the route's).
func TestSecretHandler_Get_ReadLevelGranteeReachesInspectRoute(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	var called bool
	service := &MockSecretService{mockGet: func(ctx context.Context, userID, machineID, secretID string, reveal bool) (*SecretInspectResponse, error) {
		called = true
		return &SecretInspectResponse{SecretListResponse: SecretListResponse{ID: "3f2e1d0c", Name: "db-password"}}, nil
	}}
	router := secretsAccessRouter(machineRepo, accessRepo, service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/secrets/db-password", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "a read-level grantee must reach the inspect endpoint")
	assert.True(t, called, "the inspect handler must run for a read-level grantee")
	assert.NotContains(t, w.Body.String(), "secretData",
		"the metadata answer must not carry a secretData field")
}

// --- Create (POST /api/machines/{id}/secrets) ---

// newCreateRequest builds a POST secrets request with the given JSON body,
// carrying the given user ID and machine ID the way the mounted route
// would extract them.
func newCreateRequest(userID, machineID, body string) *http.Request {
	req := httptest.NewRequest("POST", "/api/machines/"+machineID+"/secrets", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	if userID != "" {
		ctx = context.WithValue(ctx, contextutils.UserIDContextKey, userID)
	}
	return req.WithContext(ctx)
}

// TestSecretHandler_Create_HappyPath pins the happy path AND the
// handler→service seam: 201 with only {id} in the body, the authenticated
// user ID and machine ID forwarded, the name forwarded TRIMMED, and the
// value forwarded byte-identical (significant whitespace intact).
func TestSecretHandler_Create_HappyPath(t *testing.T) {
	var gotUser, gotMachine string
	var gotReq SecretCreateRequest
	mockService := &MockSecretService{
		mockCreate: func(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
			gotUser, gotMachine, gotReq = userID, machineID, req
			return &SecretCreateResponse{ID: "3f2e1d0c"}, nil
		},
	}
	handler := NewSecretHandler(mockService)

	rr := httptest.NewRecorder()
	handler.Create(rr, newCreateRequest("user123", "machine123",
		`{"name":"  db-password  ","value":"  hunter2 \nnext line\n"}`))

	assert.Equal(t, http.StatusCreated, rr.Code, "a successful create must answer 201")
	assert.JSONEq(t, `{"id":"3f2e1d0c"}`, rr.Body.String(),
		"the create response must carry the created ID only — never the value")
	assert.Equal(t, "user123", gotUser, "handler must forward the authenticated user ID")
	assert.Equal(t, "machine123", gotMachine, "handler must forward the machine ID URL param")
	assert.Equal(t, "db-password", gotReq.Name, "handler must trim the name before forwarding")
	assert.Equal(t, "  hunter2 \nnext line\n", gotReq.Value,
		"handler must forward the value byte-identical — never trimmed")
}

// TestSecretHandler_Create_ValidationFailures pins the 400 contract: a
// missing/oversized/pattern-breaking name or an empty/oversized value is
// refused with VALIDATION_FAILED before the service runs, and a malformed
// body is refused with INVALID_REQUEST.
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
		{name: "malformed body", body: `{"name":`, expectCode: "INVALID_REQUEST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			mockService := &MockSecretService{
				mockCreate: func(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
					called = true
					return &SecretCreateResponse{ID: "3f2e1d0c"}, nil
				},
			}
			handler := NewSecretHandler(mockService)

			rr := httptest.NewRecorder()
			handler.Create(rr, newCreateRequest("user123", "machine123", tt.body))

			assert.Equal(t, http.StatusBadRequest, rr.Code, "invalid input must be refused with 400")
			assert.Contains(t, rr.Body.String(), tt.expectCode, "error body should carry the %s code", tt.expectCode)
			assert.False(t, called, "an invalid request must never reach the service")
		})
	}
}

// TestSecretHandler_Create_RejectsValueWithoutEchoingIt pins the redaction
// discipline at the validation boundary: a rejected request's response (and
// thus nothing the handler could log from it) must not echo the submitted
// value — only the failing field is named.
func TestSecretHandler_Create_RejectsValueWithoutEchoingIt(t *testing.T) {
	handler := NewSecretHandler(&MockSecretService{})

	rr := httptest.NewRecorder()
	handler.Create(rr, newCreateRequest("user123", "machine123",
		`{"name":"-bad-name","value":"top-secret-payload"}`))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.NotContains(t, rr.Body.String(), "top-secret-payload",
		"a validation failure must never echo the submitted value")
}

// TestSecretHandler_Create_MissingContextAndParams pins the request-shape
// guards: no authenticated user → 401, no {id} URL param → 400.
func TestSecretHandler_Create_MissingContextAndParams(t *testing.T) {
	t.Run("missing user ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		NewSecretHandler(&MockSecretService{}).Create(rr,
			newCreateRequest("", "machine123", `{"name":"db-password","value":"hunter2"}`))
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("missing machine ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		NewSecretHandler(&MockSecretService{}).Create(rr,
			newCreateRequest("user123", "", `{"name":"db-password","value":"hunter2"}`))
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestSecretHandler_Create_ErrorMapping pins the error pass-throughs: the
// service's AppErrors keep their HTTP status (409 SECRET_ALREADY_EXISTS,
// 502 CONNECTION_FAILED) and a non-AppError falls back to 500. The 500
// SECRET_CREATION_FAILED path is asserted via status alone —
// RespondWithError rewrites 5xx bodies generic.
func TestSecretHandler_Create_ErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "duplicate name is conflict",
			mockError:      errors.ErrSecretAlreadyExists,
			expectedStatus: http.StatusConflict,
			expectedBody:   "SECRET_ALREADY_EXISTS",
		},
		{
			name:           "creation failed",
			mockError:      errors.ErrSecretCreationFailed,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "",
		},
		{
			name:           "connection failed",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:           "non-app error falls back to internal error",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockSecretService{
				mockCreate: func(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
					return nil, tt.mockError
				},
			}
			handler := NewSecretHandler(mockService)

			rr := httptest.NewRecorder()
			handler.Create(rr, newCreateRequest("user123", "machine123",
				`{"name":"db-password","value":"hunter2"}`))

			assert.Equal(t, tt.expectedStatus, rr.Code)
			if tt.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tt.expectedBody)
			}
		})
	}
}

// TestSecretHandler_Create_WriteGate pins the route mounting for the
// create route: POST /secrets is gated at write level, so a read-level
// grantee (who keeps metadata visibility) is refused with 403 in
// middleware — podman is never asked to store anything — while write- and
// admin-level grantees reach the handler.
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
			machineRepo := testutils.NewMockMachineRepository()
			machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockMachineAccessRepository()
			accessRepo.AddAccess("machine-1", "grantee-1", tt.level, time.Now())
			var called bool
			service := &MockSecretService{mockCreate: func(ctx context.Context, userID, machineID string, req SecretCreateRequest) (*SecretCreateResponse, error) {
				called = true
				return &SecretCreateResponse{ID: "3f2e1d0c"}, nil
			}}
			router := secretsAccessRouter(machineRepo, accessRepo, service)

			req := httptest.NewRequest("POST", "/api/machines/machine-1/secrets",
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

// --- Route wiring: the RequireMachineAccess subtree is the read gate ---
//
// These tests mount the secrets route on a chi subtree exactly the way
// internal/app/app.go does (RequireMachineAccess at the /machines/{id}
// subtree level, r.Get("/secrets", ...)) so the access semantics the
// contract promises are exercised through the real URL-param extraction:
// unknown machine → 404 MACHINE_NOT_FOUND, known machine without an
// access row → 403 ACCESS_DENIED, and a read-level grantee through to the
// handler (metadata is visible at read).

// secretsAccessRouter mirrors app.go's mounting of the secrets routes.
func secretsAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service SecretServiceInterface) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			requireWrite := appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)
			r.Route("/secrets", func(r chi.Router) {
				r.Get("/", NewSecretHandler(service).List)
				r.With(requireWrite).Post("/", NewSecretHandler(service).Create)
				r.Route("/{secretId}", func(r chi.Router) {
					r.Get("/", NewSecretHandler(service).Get)
					r.With(requireWrite).Delete("/", NewSecretHandler(service).Delete)
				})
			})
		})
	})
	return r
}

// requestAsUser builds a request carrying the given user ID in the
// context, standing in for JWTMiddleware.
func requestAsUser(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

func TestSecretHandler_List_UnknownMachineIsNotFound(t *testing.T) {
	var called bool
	service := &MockSecretService{mockList: func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
		called = true
		return []SecretListResponse{}, nil
	}}
	router := secretsAccessRouter(testutils.NewMockMachineRepository(), testutils.NewMockMachineAccessRepository(), service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/no-such-machine/secrets", "owner-1"))

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "unknown machine must be rejected with 404")
	assert.Contains(t, w.Body.String(), "MACHINE_NOT_FOUND", "error body should carry the MACHINE_NOT_FOUND code")
	assert.False(t, called, "handler must not run for an unknown machine")
}

func TestSecretHandler_List_UserWithoutAccessIsForbidden(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	var called bool
	service := &MockSecretService{mockList: func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
		called = true
		return []SecretListResponse{}, nil
	}}
	router := secretsAccessRouter(machineRepo, testutils.NewMockMachineAccessRepository(), service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/secrets", "stranger-1"))

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "caller without an access row must be rejected with 403")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "error body should carry the ACCESS_DENIED code")
	assert.False(t, called, "handler must not run for a caller without access")
}

func TestSecretHandler_List_ReadLevelGranteeReachesHandler(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	accessRepo := testutils.NewMockMachineAccessRepository()
	accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
	var called bool
	service := &MockSecretService{mockList: func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
		called = true
		return []SecretListResponse{}, nil
	}}
	router := secretsAccessRouter(machineRepo, accessRepo, service)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUser(http.MethodGet, "/api/machines/machine-1/secrets", "reader-1"))

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "secret metadata is visible at read level")
	assert.True(t, called, "handler must run for a read-level grantee")
}

// TestSecretHandler_Delete_SuccessAndErrorMapping pins the delete contract:
// a successful removal answers 204 with an empty body (the frontend series
// (170) invalidates the list on the 204 — there is nothing to parse), the
// service's AppErrors keep their HTTP status (404 SECRET_NOT_FOUND, 409
// SECRET_IN_USE, 502 CONNECTION_FAILED), and a non-AppError falls back to
// 500.
func TestSecretHandler_Delete_SuccessAndErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful delete",
			expectedStatus: http.StatusNoContent,
			expectedBody:   "",
		},
		{
			name:           "unknown secret is not found",
			mockError:      errors.ErrSecretNotFound,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "SECRET_NOT_FOUND",
		},
		{
			name:           "in-use secret is a conflict",
			mockError:      errors.ErrSecretInUse,
			expectedStatus: http.StatusConflict,
			expectedBody:   "SECRET_IN_USE",
		},
		{
			name:           "connection failed",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:           "non-app error falls back to internal error",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			mockService := &MockSecretService{
				mockDelete: func(ctx context.Context, userID, machineID, secretID string) error {
					called = true
					return tt.mockError
				},
			}
			handler := NewSecretHandler(mockService)

			req := httptest.NewRequest("DELETE", "/api/machines/machine123/secrets/db-password", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "machine123")
			rctx.URLParams.Add("secretId", "db-password")
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Delete(rr, req)

			assert.True(t, called, "the request must reach the service")
			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.Contains(t, rr.Body.String(), tt.expectedBody)
			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, rr.Body.String(), "a 204 must carry no body")
			}
		})
	}
}

// TestSecretHandler_Delete_MissingContextAndParams pins the request-shape
// guards for delete: no authenticated user → 401, no {secretId} URL param
// → 400.
func TestSecretHandler_Delete_MissingContextAndParams(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		secretID       string
		expectedStatus int
	}{
		{name: "missing user ID", userID: "", secretID: "db-password", expectedStatus: http.StatusUnauthorized},
		{name: "missing secret ID", userID: "user123", secretID: "", expectedStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			mockService := &MockSecretService{
				mockDelete: func(ctx context.Context, userID, machineID, secretID string) error {
					called = true
					return nil
				},
			}
			handler := NewSecretHandler(mockService)

			req := httptest.NewRequest("DELETE", "/api/machines/machine123/secrets/"+tt.secretID, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "machine123")
			rctx.URLParams.Add("secretId", tt.secretID)
			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Delete(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
			assert.False(t, called, "a malformed request must never reach the service")
		})
	}
}

// TestSecretHandler_Delete_RouteLevelGating pins the mounted-route contract
// for delete (the DELETE route rides RequireMachinePermission(write), so a
// read-level grantee is refused 403 before the handler runs, a write-level
// grantee goes through, and an unknown machine is 404'd by
// RequireMachineAccess).
func TestSecretHandler_Delete_RouteLevelGating(t *testing.T) {
	newRouter := func() (http.Handler, *int) {
		calls := 0
		service := &MockSecretService{mockDelete: func(ctx context.Context, userID, machineID, secretID string) error {
			calls++
			return nil
		}}
		machineRepo := testutils.NewMockMachineRepository()
		machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
		accessRepo := testutils.NewMockMachineAccessRepository()
		accessRepo.AddAccess("machine-1", "reader-1", models.PermissionRead, time.Now())
		accessRepo.AddAccess("machine-1", "writer-1", models.PermissionWrite, time.Now())
		return secretsAccessRouter(machineRepo, accessRepo, service), &calls
	}

	t.Run("read-level grantee is forbidden", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/machines/machine-1/secrets/db-password", "reader-1"))
		assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "a read-level grantee must be refused")
		assert.Zero(t, *calls, "a refused delete must never reach the handler")
	})

	t.Run("write-level grantee goes through", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/machines/machine-1/secrets/db-password", "writer-1"))
		assert.Equal(t, http.StatusNoContent, w.Result().StatusCode, "a write-level grantee must reach the 204")
		assert.Equal(t, 1, *calls, "the handler must run once for a write-level grantee")
	})

	t.Run("unknown machine is not found", func(t *testing.T) {
		router, calls := newRouter()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, requestAsUser(http.MethodDelete, "/api/machines/no-such-machine/secrets/db-password", "writer-1"))
		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "an unknown machine must be refused before the handler")
		assert.Zero(t, *calls, "a 404 machine must never reach the handler")
	})
}

// TestSecretHandler_Delete_InUseSurfacesPodmanMessage pins that the 409
// body carries podman's own refusal text (a 4xx, so it is NOT rewritten
// generic) — the frontend surfaces which container holds the secret.
func TestSecretHandler_Delete_InUseSurfacesPodmanMessage(t *testing.T) {
	podmanMessage := "secret db-password is in use by container 3f2e1d0c"
	mockService := &MockSecretService{
		mockDelete: func(ctx context.Context, userID, machineID, secretID string) error {
			return errors.ErrSecretInUse.WithMessage(podmanMessage)
		},
	}
	handler := NewSecretHandler(mockService)

	req := httptest.NewRequest("DELETE", "/api/machines/machine123/secrets/db-password", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine123")
	rctx.URLParams.Add("secretId", "db-password")
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	req = req.WithContext(context.WithValue(ctx, contextutils.UserIDContextKey, "user123"))

	rr := httptest.NewRecorder()
	handler.Delete(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "SECRET_IN_USE")
	assert.Contains(t, rr.Body.String(), podmanMessage,
		"the 409 body must surface podman's refusal message")
}
