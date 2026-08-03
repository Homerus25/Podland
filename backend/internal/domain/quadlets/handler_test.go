package quadlets

import (
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
	mockList func(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
}

func (m *MockQuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	if m.mockList != nil {
		return m.mockList(ctx, userID, machineID, filters)
	}
	return nil, nil
}