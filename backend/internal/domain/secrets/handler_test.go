package secrets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
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
	mockList func(ctx context.Context, userID, machineID string) ([]SecretListResponse, error)
}

func (m *MockSecretService) List(ctx context.Context, userID, machineID string) ([]SecretListResponse, error) {
	if m.mockList != nil {
		return m.mockList(ctx, userID, machineID)
	}
	return nil, nil
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

// --- Route wiring: the RequireMachineAccess subtree is the read gate ---
//
// These tests mount the secrets route on a chi subtree exactly the way
// internal/app/app.go does (RequireMachineAccess at the /machines/{id}
// subtree level, r.Get("/secrets", ...)) so the access semantics the
// contract promises are exercised through the real URL-param extraction:
// unknown machine → 404 MACHINE_NOT_FOUND, known machine without an
// access row → 403 ACCESS_DENIED, and a read-level grantee through to the
// handler (metadata is visible at read).

// secretsAccessRouter mirrors app.go's mounting of the secrets route.
func secretsAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service SecretServiceInterface) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			r.Get("/secrets", NewSecretHandler(service).List)
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
