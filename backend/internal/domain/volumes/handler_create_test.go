package volumes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// createRequest serves POST /api/machines/{id}/volumes with the given
// JSON body through the handler with an authenticated test identity.
func createRequest(create http.HandlerFunc, body string) *httptest.ResponseRecorder {
	return testutils.Perform(create, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/test-machine/volumes",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine"},
		Body:      strings.NewReader(body),
	})
}

func TestVolumeHandler_Create_Success(t *testing.T) {
	cfg := namedVolumeFixture("2026-08-30T12:00:00Z")
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}

	var createdOpts entities.VolumeCreateOptions
	engine := &fakeVolumeEngine{
		createFn: func(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
			createdOpts = opts
			return &entities.IDOrNameResponse{IDOrName: "my-volume"}, nil
		},
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return report, nil
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{"name":"my-volume","labels":{"app":"test"},"options":{"type":"tmpfs"}}`)

	assert.Equal(t, 201, w.Code, "create should return 201")
	assert.Equal(t, "my-volume", createdOpts.Name, "the decoded request must reach the machine")
	assert.Equal(t, map[string]string{"app": "test"}, createdOpts.Labels, "the decoded labels must reach the machine")
	assert.Equal(t, map[string]string{"type": "tmpfs"}, createdOpts.Options, "the decoded options must reach the machine")
	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got), "response must decode as a volume config")
	assert.Equal(t, "my-volume", got.Name)
	assert.Equal(t, "local", got.Driver)
	assert.Equal(t, "2026-08-30T12:00:00Z", got.CreatedAt)
	assert.Equal(t, map[string]string{"app": "test"}, got.Labels)
}

func TestVolumeHandler_Create_Anonymous(t *testing.T) {
	cfg := anonymousVolumeFixture()
	report := &entities.VolumeInspectReport{VolumeConfigResponse: &cfg}

	var createdOpts entities.VolumeCreateOptions
	engine := &fakeVolumeEngine{
		createFn: func(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
			createdOpts = opts
			return &entities.IDOrNameResponse{IDOrName: cfg.Name}, nil
		},
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return report, nil
		},
	}

	w := createRequest(newTestHandler(engine).Create, `{}`)

	assert.Equal(t, 201, w.Code)
	assert.Empty(t, createdOpts.Name, "an empty request name must stay empty: podman generates the name")
	var got VolumeInspectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Anonymous, "anonymous volume must be flagged")
	assert.NotEmpty(t, got.Name, "generated name must be present in the response")
}
