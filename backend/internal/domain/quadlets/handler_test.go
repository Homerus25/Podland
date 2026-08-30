package quadlets

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
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

// TestQuadletHandler_List_ForwardsQueryFiltersToService pins the HTTP seam of
// the list filters: the `resource` and `name` query parameters (contract at
// doku/description_api.md:831-833) must reach the service as ListFilters.
// TestQuadletHandler_List returns canned payloads regardless of the filters
// argument, so forwarding is asserted separately here by capturing what the
// mock service actually receives.
func TestQuadletHandler_List_ForwardsQueryFiltersToService(t *testing.T) {
	tests := []struct {
		name        string
		queryParams string
		wantFilters ListFilters
	}{
		{
			name:        "resource only",
			queryParams: "?resource=container",
			wantFilters: ListFilters{Resource: "container"},
		},
		{
			name:        "name only",
			queryParams: "?name=web",
			wantFilters: ListFilters{Name: "web"},
		},
		{
			name:        "resource and name combined",
			queryParams: "?resource=volume&name=data",
			wantFilters: ListFilters{Resource: "volume", Name: "data"},
		},
		{
			name:        "no query params yield empty filters",
			queryParams: "",
			wantFilters: ListFilters{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotFilters ListFilters
			mockService := &MockQuadletService{
				mockList: func(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
					gotFilters = filters
					return []QuadletListResponse{}, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("GET", "/api/machines/machine123/quadlets"+tt.queryParams, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", "machine123")

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, "user123")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.List(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.wantFilters, gotFilters,
				"handler must forward the resource/name query params to the service as ListFilters")
		})
	}
}

type MockQuadletService struct {
	mockList              func(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
	mockGet               func(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error)
	mockCreate            func(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error)
	mockUpdate            func(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error)
	mockDelete            func(ctx context.Context, userID, machineID, name string, force, stop bool) error
	mockStart             func(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error)
	mockStop              func(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error)
	mockRestart           func(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error)
	mockGetLogs           func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error)
	mockGetLogsWithStream func(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error
	mockGenerate          func(ctx context.Context, userID, machineID string, req QuadletGenerateRequest) (*QuadletGenerateResponse, error)
	mockImport            func(ctx context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error)
	mockAutoUpdate        func(ctx context.Context, userID, machineID string, dryRun bool, onLine func(sshexec.OutputLine)) (AutoUpdateReport, error)
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

func (m *MockQuadletService) Start(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error) {
	if m.mockStart != nil {
		return m.mockStart(ctx, userID, machineID, name)
	}
	return nil, nil
}

func (m *MockQuadletService) Stop(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	if m.mockStop != nil {
		return m.mockStop(ctx, userID, machineID, name, opts)
	}
	return nil, nil
}

func (m *MockQuadletService) Restart(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	if m.mockRestart != nil {
		return m.mockRestart(ctx, userID, machineID, name, opts)
	}
	return nil, nil
}

func (m *MockQuadletService) GetLogs(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
	if m.mockGetLogs != nil {
		return m.mockGetLogs(ctx, userID, machineID, quadletName, since, tail)
	}
	return nil, nil
}

func (m *MockQuadletService) GetLogsWithStream(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error {
	if m.mockGetLogsWithStream != nil {
		return m.mockGetLogsWithStream(ctx, userID, machineID, quadletName, since, tail, logCallback)
	}
	return nil
}

func (m *MockQuadletService) Generate(ctx context.Context, userID, machineID string, req QuadletGenerateRequest) (*QuadletGenerateResponse, error) {
	if m.mockGenerate != nil {
		return m.mockGenerate(ctx, userID, machineID, req)
	}
	return nil, nil
}

func (m *MockQuadletService) Import(ctx context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
	if m.mockImport != nil {
		return m.mockImport(ctx, userID, machineID, req)
	}
	return nil, nil
}

func (m *MockQuadletService) AutoUpdate(ctx context.Context, userID, machineID string, dryRun bool, onLine func(sshexec.OutputLine)) (AutoUpdateReport, error) {
	if m.mockAutoUpdate != nil {
		return m.mockAutoUpdate(ctx, userID, machineID, dryRun, onLine)
	}
	return AutoUpdateReport{}, nil
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

func TestQuadletHandler_Start(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletID      string
		mockResponse   *QuadletLifecycleResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:      "successful start returns 200 with running state",
			userID:    "user123",
			machineID: "machine123",
			quadletID: "my-container.container",
			mockResponse: &QuadletLifecycleResponse{
				ID:    "my-container.container",
				State: "running",
				Unit:  "my-container.service",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-container.container","state":"running","unit":"my-container.service"}` + "\n",
		},
		{
			name:           "already running maps to 400",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrQuadletAlreadyRunning,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "missing.container",
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedName string
			mockService := &MockQuadletService{
				mockStart: func(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error) {
					capturedName = name
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("POST", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID+"/start", nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Start(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
			}
		})
	}
}

func TestQuadletHandler_Stop(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		machineID       string
		quadletID       string
		queryParams     string
		mockResponse    *QuadletLifecycleResponse
		mockError       error
		expectedStatus  int
		expectedBody    string
		expectedForce   bool
		expectedTimeout int
	}{
		{
			name:        "successful stop returns 200 with stopped state",
			userID:      "user123",
			machineID:   "machine123",
			quadletID:   "my-container.container",
			queryParams: "",
			mockResponse: &QuadletLifecycleResponse{
				ID:    "my-container.container",
				State: "stopped",
				Unit:  "my-container.service",
			},
			expectedStatus:  http.StatusOK,
			expectedBody:    `{"id":"my-container.container","state":"stopped","unit":"my-container.service"}` + "\n",
			expectedForce:   false,
			expectedTimeout: 0,
		},
		{
			name:            "force=true query honoured",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?force=true",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "stopped", Unit: "my-container.service"},
			expectedStatus:  http.StatusOK,
			expectedForce:   true,
			expectedTimeout: 0,
		},
		{
			name:            "timeout=10 query honoured",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?timeout=10",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "stopped", Unit: "my-container.service"},
			expectedStatus:  http.StatusOK,
			expectedForce:   false,
			expectedTimeout: 10,
		},
		{
			name:            "force and timeout combined",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?force=true&timeout=5",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "stopped", Unit: "my-container.service"},
			expectedStatus:  http.StatusOK,
			expectedForce:   true,
			expectedTimeout: 5,
		},
		{
			name:            "invalid timeout is ignored",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?timeout=notanint",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "stopped", Unit: "my-container.service"},
			expectedStatus:  http.StatusOK,
			expectedForce:   false,
			expectedTimeout: 0,
		},
		{
			name:            "negative timeout is ignored",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?timeout=-1",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "stopped", Unit: "my-container.service"},
			expectedStatus:  http.StatusOK,
			expectedForce:   false,
			expectedTimeout: 0,
		},
		{
			name:           "not running maps to 400",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrQuadletNotRunning,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "missing.container",
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing quadlet ID",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedOpts LifecycleOptions
			var capturedName string
			mockService := &MockQuadletService{
				mockStop: func(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
					capturedOpts = opts
					capturedName = name
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("POST", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID+"/stop"+tt.queryParams, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Stop(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.expectedForce, capturedOpts.Force, "force flag should match expected")
				assert.Equal(t, tt.expectedTimeout, capturedOpts.Timeout, "timeout should match expected")
				assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
			}
		})
	}
}

func TestQuadletHandler_Restart(t *testing.T) {
	tests := []struct {
		name            string
		userID          string
		machineID       string
		quadletID       string
		queryParams     string
		mockResponse    *QuadletLifecycleResponse
		mockError       error
		expectedStatus  int
		expectedForce   bool
		expectedTimeout int
	}{
		{
			name:      "successful restart returns 200 with running state and restartedAt",
			userID:    "user123",
			machineID: "machine123",
			quadletID: "my-container.container",
			mockResponse: &QuadletLifecycleResponse{
				ID:          "my-container.container",
				State:       "running",
				Unit:        "my-container.service",
				RestartedAt: "2026-08-07T12:00:00Z",
			},
			expectedStatus:  http.StatusOK,
			expectedForce:   false,
			expectedTimeout: 0,
		},
		{
			name:            "force and timeout honoured",
			userID:          "user123",
			machineID:       "machine123",
			quadletID:       "my-container.container",
			queryParams:     "?force=true&timeout=20",
			mockResponse:    &QuadletLifecycleResponse{ID: "my-container.container", State: "running", Unit: "my-container.service", RestartedAt: "2026-08-07T12:00:00Z"},
			expectedStatus:  http.StatusOK,
			expectedForce:   true,
			expectedTimeout: 20,
		},
		{
			name:           "not running maps to 400",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrQuadletNotRunning,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "missing.container",
			mockError:      errors.ErrQuadletNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "my-container.container",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing quadlet ID",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedOpts LifecycleOptions
			var capturedName string
			mockService := &MockQuadletService{
				mockRestart: func(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
					capturedOpts = opts
					capturedName = name
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("POST", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID+"/restart"+tt.queryParams, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Restart(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.expectedForce, capturedOpts.Force, "force flag should match expected")
				assert.Equal(t, tt.expectedTimeout, capturedOpts.Timeout, "timeout should match expected")
				assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
				// Restart success body must carry a restartedAt timestamp.
				assert.Contains(t, rr.Body.String(), "restartedAt", "restart response must include restartedAt")
			}
		})
	}
}

func TestQuadletHandler_Generate(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		machineID      string
		quadletID      string
		requestBody    string
		mockResponse   *QuadletGenerateResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:        "successful generate from container",
			userID:      "user123",
			machineID:   "machine123",
			quadletID:   "generate",
			requestBody: `{"resourceId":"my-container","resourceType":"container"}`,
			mockResponse: &QuadletGenerateResponse{
				ID:       "my-container",
				Name:     "my-container",
				Resource: "container",
				State:    "unknown",
				Content:  "# Generated by Podland from container my-container\n[Unit]\nDescription=Podman container my-container\n\n[Container]\nContainerName=my-container\nImage=nginx:latest\n",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-container","name":"my-container","resource":"container","state":"unknown","content":"# Generated by Podland from container my-container\n[Unit]\nDescription=Podman container my-container\n\n[Container]\nContainerName=my-container\nImage=nginx:latest\n"}` + "\n",
		},
		{
			name:        "successful generate from pod",
			userID:      "user123",
			machineID:   "machine123",
			quadletID:   "generate",
			requestBody: `{"resourceId":"my-pod","resourceType":"pod"}`,
			mockResponse: &QuadletGenerateResponse{
				ID:       "my-pod",
				Name:     "my-pod",
				Resource: "pod",
				State:    "unknown",
				Content:  "# Generated by Podland from pod my-pod\n[Unit]\nDescription=Podman pod my-pod\n\n[Pod]\nPodName=my-pod\n",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":"my-pod","name":"my-pod","resource":"pod","state":"unknown","content":"# Generated by Podland from pod my-pod\n[Unit]\nDescription=Podman pod my-pod\n\n[Pod]\nPodName=my-pod\n"}` + "\n",
		},
		{
			name:           "validation failure: missing resourceId",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceType":"container"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: empty resourceId",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"","resourceType":"container"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: missing resourceType",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: empty resourceType",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "validation failure: invalid resourceType",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-resource","resourceType":"invalid"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "container not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"missing-container","resourceType":"container"}`,
			mockError:      errors.ErrContainerNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "pod not found maps to 404",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"missing-pod","resourceType":"pod"}`,
			mockError:      errors.ErrPodNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "quadlet generation failed maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":"container"}`,
			mockError:      errors.ErrQuadletGenerationFailed,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "connection failed maps to 502",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":"container"}`,
			mockError:      errors.ErrConnectionFailed,
			expectedStatus: http.StatusBadGateway,
		},
		{
			name:           "generic error maps to 500",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":"container"}`,
			mockError:      errGeneric,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "missing user ID",
			userID:         "",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":"container"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "missing machine ID",
			userID:         "user123",
			machineID:      "",
			quadletID:      "generate",
			requestBody:    `{"resourceId":"my-container","resourceType":"container"}`,
			expectedStatus: http.StatusBadRequest,
		},

		{
			name:           "malformed JSON body",
			userID:         "user123",
			machineID:      "machine123",
			quadletID:      "generate",
			requestBody:    `{not valid json`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedReq QuadletGenerateRequest
			mockService := &MockQuadletService{
				mockGenerate: func(ctx context.Context, userID, machineID string, req QuadletGenerateRequest) (*QuadletGenerateResponse, error) {
					capturedReq = req
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewQuadletHandler(mockService, nil)

			req := httptest.NewRequest("POST", "/api/machines/"+tt.machineID+"/quadlets/"+tt.quadletID, bytes.NewBufferString(tt.requestBody))

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.machineID)
			rctx.URLParams.Add("quadletId", tt.quadletID)

			ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
			ctx = context.WithValue(ctx, contextutils.UserIDContextKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler.Generate(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				actualBody := rr.Body.String()
				if actualBody != tt.expectedBody {
					t.Errorf("handler returned unexpected body: got %q want %q", actualBody, tt.expectedBody)
				}
			}

			// On success, verify the request was passed through correctly
			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.requestBody, `{"resourceId":"`+capturedReq.ResourceID+`","resourceType":"`+capturedReq.ResourceType+`"}`, "request should be passed through correctly")
			}
		})
	}
}
