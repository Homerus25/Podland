package quadlets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/contextutils"
	"github.com/yourorg/podland/backend/internal/errors"
)

func TestHandlerImport_Success(t *testing.T) {
	t.Parallel()

	importResult := &QuadletImportResponse{
		Imported: []QuadletImportResult{
			{ID: "web", Name: "web", Path: "/etc/containers/systemd/web.container", State: "unknown"},
			{ID: "db", Name: "db", Path: "/etc/containers/systemd/db.container", State: "unknown"},
		},
		Failed: []QuadletImportError{},
	}

	mockService := &MockQuadletService{
		mockImport: func(_ context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
			return importResult, nil
		},
	}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
			{Name: "db", Resource: "container", Content: "[Container]\nImage=postgres\n"},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader(body))

	// Set up chi URL parameters
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response QuadletImportResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Imported, 2)
	assert.Len(t, response.Failed, 0)
	assert.Equal(t, "web", response.Imported[0].Name)
	assert.Equal(t, "db", response.Imported[1].Name)
}

func TestHandlerImport_InvalidCredentials(t *testing.T) {
	t.Parallel()

	mockService := &MockQuadletService{}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandlerImport_MissingMachineID(t *testing.T) {
	t.Parallel()

	mockService := &MockQuadletService{}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines//quadlets/import", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerImport_InvalidRequestBody(t *testing.T) {
	t.Parallel()

	mockService := &MockQuadletService{}
	handler := NewQuadletHandler(mockService, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader([]byte("invalid json")))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerImport_ServiceError(t *testing.T) {
	t.Parallel()

	mockService := &MockQuadletService{
		mockImport: func(_ context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
			return nil, errors.ErrConnectionFailed
		},
	}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandlerImport_EmptyQuadletsList(t *testing.T) {
	t.Parallel()

	mockService := &MockQuadletService{}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandlerImport_PartialFailure(t *testing.T) {
	t.Parallel()

	importResult := &QuadletImportResponse{
		Imported: []QuadletImportResult{
			{ID: "web", Name: "web", Path: "/etc/containers/systemd/web.container", State: "unknown"},
		},
		Failed: []QuadletImportError{
			{Name: "db", Error: "quadlet already exists"},
		},
	}

	mockService := &MockQuadletService{
		mockImport: func(_ context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
			return importResult, nil
		},
	}
	handler := NewQuadletHandler(mockService, nil)

	reqBody := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
			{Name: "db", Resource: "container", Content: "[Container]\nImage=postgres\n"},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.Import(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response QuadletImportResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Imported, 1)
	assert.Len(t, response.Failed, 1)
	assert.Equal(t, "web", response.Imported[0].Name)
	assert.Equal(t, "db", response.Failed[0].Name)
	assert.Contains(t, response.Failed[0].Error, "already exists")
}
