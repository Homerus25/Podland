package quadlets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

func importRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/machines/machine-1/quadlets/import", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "machine-1")
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = contextutils.SetUserID(ctx, "user-1")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	return req.WithContext(ctx)
}

const twoQuadletsBody = `{"quadlets":[
	{"name":"web","resource":"container","content":"[Container]\nImage=nginx\n"},
	{"name":"db","resource":"container","content":"[Container]\nImage=postgres\n"}]}`

func TestHandlerImport_Success(t *testing.T) {
	t.Parallel()

	var installed []string
	engine := &fakeQuadletEngine{
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			installed = append(installed, fileName)
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"src": "/etc/containers/systemd/" + fileName},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	newTestHandler(engine).Import(w, importRequest(t, twoQuadletsBody))

	require.Equal(t, http.StatusCreated, w.Code)

	var response QuadletImportResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))

	assert.Len(t, response.Imported, 2)
	assert.Len(t, response.Failed, 0)
	assert.Equal(t, "web", response.Imported[0].Name)
	assert.Equal(t, "db", response.Imported[1].Name)
	assert.ElementsMatch(t, []string{"web.container", "db.container"}, installed,
		"each entry must be installed under its resource-derived filename")
}

func TestHandlerImport_EmptyQuadletsList(t *testing.T) {
	t.Parallel()

	engine := &fakeQuadletEngine{}

	w := httptest.NewRecorder()
	newTestHandler(engine).Import(w, importRequest(t, `{"quadlets":[]}`))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestHandlerImport_PartialFailure pins that a per-entry install failure is
// data in the response (201 with a Failed entry), not a failed request — the
// other entries still land on the machine.
func TestHandlerImport_PartialFailure(t *testing.T) {
	t.Parallel()

	engine := &fakeQuadletEngine{
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			if fileName == "db.container" {
				return nil, assert.AnError
			}
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"src": "/etc/containers/systemd/" + fileName},
			}, nil
		},
	}

	w := httptest.NewRecorder()
	newTestHandler(engine).Import(w, importRequest(t, twoQuadletsBody))

	require.Equal(t, http.StatusCreated, w.Code)

	var response QuadletImportResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))

	require.Len(t, response.Imported, 1)
	assert.Equal(t, "web", response.Imported[0].Name)
	require.Len(t, response.Failed, 1)
	assert.Equal(t, "db", response.Failed[0].Name)
	assert.NotEmpty(t, response.Failed[0].Error, "the failure must carry a reason")
}
