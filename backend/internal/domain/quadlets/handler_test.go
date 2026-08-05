package quadlets

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
)

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
			name:      "successful list",
			userID:    "user123",
			machineID: "machine123",
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
			name:      "filter by resource",
			userID:    "user123",
			machineID: "machine123",
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
			name:      "filter by name",
			userID:    "user123",
			machineID: "machine123",
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