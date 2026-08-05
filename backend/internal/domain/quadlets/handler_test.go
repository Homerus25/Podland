package quadlets

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
)

// errGeneric is a non-AppError used to exercise the handler's generic
// error→500 mapping in TestQuadletHandler_Delete and TestQuadletHandler_Update.
// The app errors package has no New constructor, so we synthesize a plain error
// here.
var errGeneric = fmt.Errorf("boom")

func TestQuadletHandler_List(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		queryParams    string
		mockQuadlets   []QuadletListResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:        "successful list",
			userID:      "user123",
			machineID:   "machine123",
			queryParams: "",
			mockQuadlets: []QuadletListResponse{
				{
					ID:       "my-container.container",
					Name:     "my-container.container",
					Path:     "/etc/containers/systemd/my-container.container",
					Unit:     "my-container.service",
					Resource: "container",
					State:    "running",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"running"}]` + "\n",
		},
		{
			name:           "successful list empty",
			userID:         "user123",
			machineID:      "machine123",
			queryParams:    "",
			mockQuadlets:   []QuadletListResponse{},
			expectedStatus: http.StatusOK,
			expectedBody:   `[]` + "\n",
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			queryParams:    "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "",
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "",
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			queryParams:    "",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:        "filter by resource",
			userID:      "user123",
			machineID:   "machine123",
			queryParams: "?resource=container",
			mockQuadlets: []QuadletListResponse{
				{
					ID:       "my-container.container",
					Name:     "my-container.container",
					Path:     "/etc/containers/systemd/my-container.container",
					Unit:     "my-container.service",
					Resource: "container",
					State:    "running",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"running"}]` + "\n",
		},
		{
			name:        "filter by name",
			userID:      "user123",
			machineID:   "machine123",
			queryParams: "?name=pod",
			mockQuadlets: []QuadletListResponse{
				{
					ID:       "my-pod.pod",
					Name:     "my-pod.pod",
					Path:     "/etc/containers/systemd/my-pod.pod",
					Unit:     "my-pod.service",
					Resource: "pod",
					State:    "stopped",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `[{"id":"my-pod.pod","name":"my-pod.pod","path":"/etc/containers/systemd/my-pod.pod","unit":"my-pod.service","resource":"pod","state":"stopped"}]` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockQuadletService{
				mockList: func(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockQuadlets, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("GET", "/api/machines/"+tt.machineID+"/quadlets"+tt.queryParams, nil)

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

type MockQuadletService struct {
	mockList   func(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
	mockGet    func(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error)
	mockCreate func(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error)
	mockUpdate func(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error)
	mockDelete func(ctx context.Context, userID, machineID, name string, force, stop bool) error
}

func (m *MockQuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	if m.mockList != nil {
		return m.mockList(ctx, userID, machineID, filters)
	}
	return nil, nil
}

func (m *MockQuadletService) Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error) {
	if m.mockGet != nil {
		return m.mockGet(ctx, userID, machineID, name)
	}
	return nil, nil
}

func (m *MockQuadletService) Create(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error) {
	if m.mockCreate != nil {
		return m.mockCreate(ctx, userID, machineID, req)
	}
	return nil, nil
}

func (m *MockQuadletService) Update(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error) {
	if m.mockUpdate != nil {
		return m.mockUpdate(ctx, userID, machineID, name, req)
	}
	return nil, nil
}

func (m *MockQuadletService) Delete(ctx context.Context, userID, machineID, name string, force, stop bool) error {
	if m.mockDelete != nil {
		return m.mockDelete(ctx, userID, machineID, name, force, stop)
	}
	return nil
}

func TestQuadletHandler_Get(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletID      string
		mockQuadlet    *QuadletGetResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:      "successful get",
			userID:    "user123",
			machineID: "machine123",
			quadletID: "my-container.container",
			mockQuadlet: &QuadletGetResponse{
				ID:       "my-container.container",
				Name:     "my-container.container",
				Path:     "/etc/containers/systemd/my-container.container",
				Unit:     "my-container.service",
				Resource: "container",
				State:    "running",
				Content:  "[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"running","content":"[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n"}` + "\n",
		},
		{
			name:           "quadlet not found",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "nonexistent.container",
			mockQuadlet:    nil,
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockQuadlet:    nil,
			mockError:      nil,
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "",
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			quadletID:      "my-container.container",
			mockQuadlet:    nil,
			mockError:      nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "",
		},
		{
			name:           "missing quadlet ID",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "",
			mockQuadlet:    nil,
			mockError:      nil,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "",
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockQuadlet:    nil,
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
			expectedBody:   "",
		},
		{
			name:      "pod resource type",
			userID:    "user123",
			machineID: "machine123",
			quadletID: "my-pod.pod",
			mockQuadlet: &QuadletGetResponse{
				ID:       "my-pod.pod",
				Name:     "my-pod.pod",
				Path:     "/etc/containers/systemd/my-pod.pod",
				Unit:     "my-pod.service",
				Resource: "pod",
				State:    "stopped",
				Content:  "[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-pod.pod","name":"my-pod.pod","path":"/etc/containers/systemd/my-pod.pod","unit":"my-pod.service","resource":"pod","state":"stopped","content":"[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n"}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockQuadletService{
				mockGet: func(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockQuadlet, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("GET", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Get(rr, req)

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

func TestQuadletHandler_Create(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		requestBody    string
		mockResponse   *QuadletCreateResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:        "successful create",
			userID:      "user123",
			machineID:   "machine123",
			requestBody: `{"name":"my-container","resource":"container","content":"[Container]\nImage=nginx:latest\n"}`,
			mockResponse: &QuadletCreateResponse{
				ID:       "my-container",
				Name:     "my-container",
				Path:     "/etc/containers/systemd/my-container.container",
				Resource: "container",
				State:    "unknown",
				Content:  "[Container]\nImage=nginx:latest\n",
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   `{"id":"my-container","name":"my-container","path":"/etc/containers/systemd/my-container.container","resource":"container","state":"unknown","content":"[Container]\nImage=nginx:latest\n"}` + "\n",
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			requestBody:    `{"name":"x","resource":"container","content":"y"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			requestBody:    `{"name":"x","resource":"container","content":"y"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed JSON body",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{not valid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: missing name",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{"resource":"container","content":"y"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: invalid resource",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{"name":"x","resource":"kube","content":"y"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: missing content",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{"name":"x","resource":"container"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "conflict (already exists)",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{"name":"dup","resource":"container","content":"y"}`,
			mockError:      errors.ErrQuadletAlreadyExists,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "connection failed",
			userID:         "user123",
			machineID:      "machine123",
			requestBody:    `{"name":"x","resource":"container","content":"y"}`,
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockQuadletService{
				mockCreate: func(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("POST", "/api/machines/"+tt.machineID+"/quadlets", bytes.NewBufferString(tt.requestBody))

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Create(rr, req)

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

func TestQuadletHandler_Update(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletID      string
		requestBody    string
		mockResponse   *QuadletGetResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:        "successful update returns 200 with the updated quadlet",
			userID:      "user123",
			machineID:   "machine123",
			quadletID:   "my-container.container",
			requestBody: `{"content":"[Container]\nImage=nginx:1.25\n"}`,
			mockResponse: &QuadletGetResponse{
				ID:       "my-container.container",
				Name:     "my-container.container",
				Path:     "/etc/containers/systemd/my-container.container",
				Unit:     "my-container.service",
				Resource: "container",
				State:    "unknown",
				Content:  "[Container]\nImage=nginx:1.25\n",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"unknown","content":"[Container]\nImage=nginx:1.25\n"}` + "\n",
		},
		{
			name:           "quadlet not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "missing.container",
			requestBody:    `{"content":"[Container]\nImage=nginx:1.25\n"}`,
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "validation failure: missing content",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: empty content",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{"content":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed JSON body",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{not valid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{"content":"x"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			quadletID:      "my-container.container",
			requestBody:    `{"content":"x"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing quadlet ID",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "",
			requestBody:    `{"content":"x"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{"content":"x"}`,
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			requestBody:    `{"content":"x"}`,
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedName string
			var capturedReq QuadletUpdateRequest
			mockService := &MockQuadletService{
				mockUpdate: func(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error) {
					capturedName = name
					capturedReq = req
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("PUT", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID, bytes.NewBufferString(tt.requestBody))

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Update(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				actualBody := rr.Body.String()
				if actualBody != tt.expectedBody {
					t.Errorf("handler returned unexpected body: got %q want %q", actualBody, tt.expectedBody)
				}
			}

			// On success, verify the quadlet name and request content were
			// passed through to the service.
			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
				assert.Equal(t, "[Container]\nImage=nginx:1.25\n", capturedReq.Content, "request content should be passed through")
			}
		})
	}
}

func TestQuadletHandler_Delete(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletID      string
		queryParams    string
		mockError      error
		expectedStatus int
		expectedForce  bool
		expectedStop   bool
	}{
		{
			name:           "successful delete returns 204 with empty body",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "",
			mockError:      nil,
			expectedStatus: http.StatusNoContent,
			expectedForce:  false,
			expectedStop:   true, // default
		},
		{
			name:           "force=true query honoured",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "?force=true",
			mockError:      nil,
			expectedStatus: http.StatusNoContent,
			expectedForce:  true,
			expectedStop:   true,
		},
		{
			name:           "stop=false query honoured",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "?stop=false",
			mockError:      nil,
			expectedStatus: http.StatusNoContent,
			expectedForce:  false,
			expectedStop:   false,
		},
		{
			name:           "force=true&stop=false combined",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "?force=true&stop=false",
			mockError:      nil,
			expectedStatus: http.StatusNoContent,
			expectedForce:  true,
			expectedStop:   false,
		},
		{
			name:           "running conflict maps to 400",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "",
			mockError:      errors.ErrQuadletResourceRunning,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "missing.container",
			queryParams:    "",
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			quadletID:      "my-container.container",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing quadlet ID",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid force boolean is ignored (defaults to false)",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "?force=notabool",
			mockError:      nil,
			expectedStatus: http.StatusNoContent,
			expectedForce:  false,
			expectedStop:   true,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			queryParams:    "",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedForce, capturedStop bool
			var capturedName string
			mockService := &MockQuadletService{
				mockDelete: func(ctx context.Context, userID, machineID, name string, force, stop bool) error {
					capturedForce = force
					capturedStop = stop
					capturedName = name
					return tt.mockError
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("DELETE", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID+tt.queryParams, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Delete(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			// On success the body must be empty (204 No Content).
			if tt.expectedStatus == http.StatusNoContent && rr.Body.String() != "" {
				t.Errorf("handler returned non-empty body on 204: got %q", rr.Body.String())
			}

			// Verify the service was called with the right parsed params when
			// the request was valid (had user + machine + quadlet).
			if tt.expectedStatus == http.StatusNoContent {
				assert.Equal(t, tt.expectedForce, capturedForce, "force flag should match expected")
				assert.Equal(t, tt.expectedStop, capturedStop, "stop flag should match expected")
				assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
			}
		})
	}
}
