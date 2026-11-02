package images

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// newTestHandler builds the handler around the real service running over
// the given fake engine; tests arm only the operations their scenario
// needs (an unarmed operation panics if the handler reaches it).
func newTestHandler(engine *fakeImageEngine) *ImageHandler {
	return NewImageHandler(NewImageService(fakeEngineProvider{engine: engine}))
}

// authedContext adds the identity the router would extract from the JWT.
func authedContext() context.Context {
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	return contextutils.SetRequestID(ctx, "test-request-id")
}

// TestImageHandler_List_Success pins the list contract through the real
// service: 200, JSON array, and the DTO fields the API promises.
func TestImageHandler_List_Success(t *testing.T) {
	created := int64(1700000000)
	testImages := []*entities.ImageSummary{
		{
			ID:          "sha256:abc123",
			RepoTags:    []string{"nginx:latest"},
			RepoDigests: []string{"nginx@sha256:digest1"},
			ParentId:    "sha256:parent1",
			Created:     created,
			Size:        102400000,
			Os:          "linux",
			Arch:        "amd64",
			Labels:      map[string]string{"maintainer": "NGINX"},
		},
		{
			ID:       "sha256:def456",
			RepoTags: []string{"alpine:3.18"},
			Created:  created,
			Size:     5000000,
		},
	}
	engine := &fakeImageEngine{
		listFn: func(entities.ImageListOptions) ([]*entities.ImageSummary, error) {
			return testImages, nil
		},
	}
	handler := newTestHandler(engine)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images", nil).WithContext(authedContext())

	rr := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("GET /api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "Should return 200 OK")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var result []ImageListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	require.NoError(t, err, "Should be able to parse JSON response")
	assert.Equal(t, 2, len(result), "Should return correct number of images")

	assert.Equal(t, "sha256:abc123", result[0].ID)
	assert.Equal(t, []string{"nginx:latest"}, result[0].RepoTags)
	assert.Equal(t, []string{"nginx@sha256:digest1"}, result[0].RepoDigests)
	assert.Equal(t, "sha256:parent1", result[0].ParentID)
	assert.Equal(t, int64(102400000), result[0].Size)
	assert.Equal(t, "linux", result[0].OS)
	assert.Equal(t, "amd64", result[0].Architecture)
	assert.Equal(t, "NGINX", result[0].Labels["maintainer"])

	assert.Equal(t, "sha256:def456", result[1].ID)
	assert.Empty(t, result[1].ParentID)
	assert.Empty(t, result[1].OS)
}

// TestImageHandler_List_AllQueryParam tests that the all query parameter is parsed
// and reaches the machine's list call.
func TestImageHandler_List_AllQueryParam(t *testing.T) {
	var gotOpts entities.ImageListOptions
	engine := &fakeImageEngine{
		listFn: func(opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
			gotOpts = opts
			return []*entities.ImageSummary{}, nil
		},
	}
	handler := newTestHandler(engine)

	req := httptest.NewRequest("GET", "/api/machines/test-machine/images?all=true", nil).WithContext(authedContext())

	rr := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("GET /api/machines/{id}/images", handler.List)
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, gotOpts.All, "the all query param must reach the machine as All")
}

// TestImageHandler_List_FiltersQueryParam pins the HTTP seam of the list
// filters: the `filters` query parameter (a JSON map[string][]string, contract
// at doku/description_images_api.md:15-16) must reach the machine as flattened
// "key=value" entries in ImageListOptions.Filter. Map iteration order makes
// the flattened order non-deterministic, so the assertions use ElementsMatch.
func TestImageHandler_List_FiltersQueryParam(t *testing.T) {
	tests := []struct {
		name        string
		queryParams string
		wantFilters []string
	}{
		{
			name:        "single key with single value",
			queryParams: `?filters={"reference":["busybox:latest"]}`,
			wantFilters: []string{"reference=busybox:latest"},
		},
		{
			name:        "single key with multiple values",
			queryParams: `?filters={"label":["maintainer=x","arch=arm64"]}`,
			wantFilters: []string{"label=maintainer=x", "label=arch=arm64"},
		},
		{
			name:        "multiple keys are all flattened",
			queryParams: `?filters={"reference":["nginx"],"label":["a=1"]}`,
			wantFilters: []string{"reference=nginx", "label=a=1"},
		},
		{
			name:        "invalid filters JSON is ignored",
			queryParams: `?filters=not-json`,
			wantFilters: nil,
		},
		{
			name:        "filters combine with the all param",
			queryParams: `?all=true&filters={"dangling":["false"]}`,
			wantFilters: []string{"dangling=false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedOpts entities.ImageListOptions
			engine := &fakeImageEngine{
				listFn: func(opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
					capturedOpts = opts
					return []*entities.ImageSummary{}, nil
				},
			}
			handler := newTestHandler(engine)

			req := httptest.NewRequest("GET", "/api/machines/test-machine/images"+tt.queryParams, nil).WithContext(authedContext())

			rr := httptest.NewRecorder()
			r := http.NewServeMux()
			r.HandleFunc("GET /api/machines/{id}/images", handler.List)
			r.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, "an (even ignored) filters param must never fail the request")
			assert.ElementsMatch(t, tt.wantFilters, capturedOpts.Filter,
				"handler must flatten the filters query param into opts.Filter as key=value entries")
			if tt.name == "filters combine with the all param" {
				assert.True(t, capturedOpts.All, "the all param must still be parsed alongside filters")
			}
		})
	}
}

// TestImageHandler_Delete_Success tests successful image deletion
func TestImageHandler_Delete_Success(t *testing.T) {
	var gotID string
	var gotForce bool
	engine := &fakeImageEngine{
		removeFn: func(id string, force bool) (*entities.ImageRemoveReport, []error) {
			gotID, gotForce = id, force
			return &entities.ImageRemoveReport{}, nil
		},
	}
	handler := newTestHandler(engine)

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123", nil).WithContext(authedContext())

	w := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("DELETE /api/machines/{id}/images/{imageId}", handler.Delete)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success")
	assert.Equal(t, "sha256:abc123", gotID)
	assert.False(t, gotForce, "a missing force param must delete unforced")
}

// TestImageHandler_Delete_Force tests deletion with force query parameter
func TestImageHandler_Delete_Force(t *testing.T) {
	var gotForce bool
	engine := &fakeImageEngine{
		removeFn: func(id string, force bool) (*entities.ImageRemoveReport, []error) {
			gotForce = force
			return &entities.ImageRemoveReport{}, nil
		},
	}
	handler := newTestHandler(engine)

	req := httptest.NewRequest("DELETE", "/api/machines/test-machine/images/sha256:abc123?force=true", nil).WithContext(authedContext())

	w := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("DELETE /api/machines/{id}/images/{imageId}", handler.Delete)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "Should return 204 on success with force=true")
	assert.True(t, gotForce, "force=true must reach the machine")
}

// TestImageHandler_Pull_InvalidRequestBody tests that Pull handles invalid request body
func TestImageHandler_Pull_InvalidRequestBody(t *testing.T) {
	handler := newTestHandler(&fakeImageEngine{})

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", nil).
		WithContext(authedContext())
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("POST /api/machines/{id}/images/pull", handler.Pull)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when request body is invalid")
}

// TestImageHandler_Pull_ValidationFailure tests that Pull handles validation failures
func TestImageHandler_Pull_ValidationFailure(t *testing.T) {
	handler := newTestHandler(&fakeImageEngine{})

	requestBody := map[string]any{
		"platform": "linux/amd64",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody)).
		WithContext(authedContext())
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("POST /api/machines/{id}/images/pull", handler.Pull)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Should return 400 when validation fails")
}

// splitLines splits a string into lines, filtering out empty lines
func splitLines(s string) []string {
	lines := []string{}
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// findEvent decodes the first NDJSON line whose event carries the wanted
// type.
func findEvent(t *testing.T, body string, eventType string) PullProgressEvent {
	t.Helper()
	for _, line := range splitLines(body) {
		var event PullProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err == nil && event.Type == eventType {
			return event
		}
	}
	t.Fatalf("no %s event found in stream: %s", eventType, body)
	return PullProgressEvent{}
}

// TestImageHandler_Pull_Success_Streaming pins the pull stream through the
// real service: progress emitted inside the machine's pull surfaces as
// NDJSON events, and the stream ends with the complete event carrying the
// machine ID.
func TestImageHandler_Pull_Success_Streaming(t *testing.T) {
	engine := &fakeImageEngine{
		pullFn: func(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			require.NotNil(t, opts.Writer, "the service must arm the pull options' progress writer")
			_, err := opts.Writer.Write([]byte("layer 1 pulling"))
			require.NoError(t, err)
			return &entities.ImagePullReport{Images: []string{"sha256:abc123"}}, nil
		},
	}
	handler := newTestHandler(engine)

	requestBody := map[string]string{
		"image": "nginx:latest",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/machines/test-machine/images/pull", bytes.NewReader(jsonBody)).
		WithContext(authedContext())
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r := http.NewServeMux()
	r.HandleFunc("POST /api/machines/{id}/images/pull", handler.Pull)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 for streaming response")
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "Should set Content-Type to application/json")

	completeEvent := findEvent(t, w.Body.String(), EventTypeComplete)
	assert.Equal(t, "test-machine", completeEvent.MachineID, "Should have correct machine ID")
}
