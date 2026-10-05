package secrets

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// newMoveTestHandler builds the handler over the real service with
// per-machine fake engines, a source-machine usage fake, and the
// admin-on-source/write-on-target access store — the wiring the move
// endpoints exercise.
func newMoveTestHandler(sourceEngine, targetEngine *fakeSecretEngine, usage *fakeUsageEngine) *SecretHandler {
	service := newMoveTestService(sourceEngine, targetEngine, usage,
		&fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})
	return NewSecretHandler(service)
}

// secretMoveErrorEnvelope mirrors the {"requestId","error":{code,message}}
// shape the endpoint envelope answers failures with — only the fields the
// assertions consume are named.
type secretMoveErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// secretMoveRequest drives the move handler the way the mounted route
// would: POST with the transfer body and the route's URL params.
func secretMoveRequest(t *testing.T, handler *SecretHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return testutils.Perform(handler.Move, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/secrets/db-password/move",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "db-password"},
		Body:      strings.NewReader(body),
	})
}

func TestSecretHandler_MovePreview_ReturnsPreview(t *testing.T) {
	usage := &fakeUsageEngine{
		listQuadletNamesFn: func() ([]string, error) {
			return []string{"web.container"}, nil
		},
		printQuadletFn: func(string) (string, error) {
			return "[Container]\nImage=busybox\nSecret=db-password\n", nil
		},
	}

	w := testutils.Perform(newMoveTestHandler(moveSourceFixture(), emptyTargetFixture(), usage).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets/db-password/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "db-password"},
	})

	require.Equal(t, http.StatusOK, w.Code)

	var got SecretMovePreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.Collision, "an empty target holds no same-named secret")
	require.Len(t, got.ReferencingQuadlets, 1)
	assert.Equal(t, "web.container", got.ReferencingQuadlets[0].Name)
}

func TestSecretHandler_MovePreview_EmptyQuadletsRendersAsArray(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets/db-password/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "db-password"},
	})

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"collision":false,"referencingQuadlets":[]}`, w.Body.String(),
		"a machine without quadlets must render an empty JSON array, not null")
}

func TestSecretHandler_MovePreview_MissingSecretIDIs400(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(&fakeSecretEngine{}, &fakeSecretEngine{}, idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets//move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": ""},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSecretHandler_MovePreview_MissingTargetMachineIDIs400(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(&fakeSecretEngine{}, &fakeSecretEngine{}, idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets/db-password/move-preview",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "db-password"},
	})

	require.Equal(t, http.StatusBadRequest, w.Code)

	var errResp secretMoveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "MISSING_TARGET_MACHINE_ID", errResp.Error.Code)
}

func TestSecretHandler_MovePreview_TargetAccessDeniedIs403(t *testing.T) {
	// The handler drives the real service, so the 403 comes from the
	// service's target gate: the access store answers with no row and the
	// source engine's inspect is armed to panic, proving the source lease
	// never opened.
	source := &fakeSecretEngine{
		inspectFn: func(string, bool) (*entities.SecretInfoReport, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newMoveTestService(source, &fakeSecretEngine{}, idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: func(machineID, userID string) (string, error) {
			if machineID == "machine-b" {
				return "", nil
			}
			return "admin", nil
		}})
	handler := NewSecretHandler(service)

	w := testutils.Perform(handler.MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets/db-password/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "db-password"},
	})

	require.Equal(t, http.StatusForbidden, w.Code)

	var errResp secretMoveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}

func TestSecretHandler_MovePreview_MissingSourceSecretIs404(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/secrets/no-such-secret/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": "no-such-secret"},
	})

	require.Equal(t, http.StatusNotFound, w.Code)

	var errResp secretMoveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "SECRET_NOT_FOUND", errResp.Error.Code)
}

// --- Move (POST /api/machines/{id}/secrets/{secretId}/move) ---------------

func TestSecretHandler_Move_ReturnsEnvelope(t *testing.T) {
	w := secretMoveRequest(t, newMoveTestHandler(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture()),
		`{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusOK, w.Code)

	var got SecretMoveResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "copied", got.Status)
	assert.Equal(t, "db-password", got.Name)
	assert.Equal(t, "machine-a", got.SourceMachineID)
	assert.Equal(t, "machine-b", got.TargetMachineID)
	assert.Empty(t, got.Error)
	assert.NotContains(t, w.Body.String(), "secretData",
		"the transfer envelope must never carry the value")
}

func TestSecretHandler_Move_MoveModeEnvelopeIsMoved(t *testing.T) {
	w := secretMoveRequest(t, newMoveTestHandler(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture()),
		`{"targetMachineId":"machine-b","mode":"move"}`)

	require.Equal(t, http.StatusOK, w.Code)

	var got SecretMoveResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "moved", got.Status)
}

func TestSecretHandler_Move_MissingSecretIDIs400(t *testing.T) {
	handler := newMoveTestHandler(&fakeSecretEngine{}, &fakeSecretEngine{}, idleUsageFixture())

	w := testutils.Perform(handler.Move, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/secrets//move",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "secretId": ""},
		Body:      strings.NewReader(`{"targetMachineId":"machine-b","mode":"copy"}`),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSecretHandler_Move_BodyValidationIs400(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed JSON", body: `{`, want: "INVALID_REQUEST"},
		{name: "missing targetMachineId", body: `{"mode":"copy"}`, want: "VALIDATION_FAILED"},
		{name: "mode other than copy/move", body: `{"targetMachineId":"machine-b","mode":"teleport"}`, want: "VALIDATION_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newMoveTestHandler(&fakeSecretEngine{}, &fakeSecretEngine{}, idleUsageFixture())

			w := secretMoveRequest(t, handler, tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)

			var errResp secretMoveErrorEnvelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
			assert.Equal(t, tt.want, errResp.Error.Code)
		})
	}
}

func TestSecretHandler_Move_CollisionIs409(t *testing.T) {
	target := emptyTargetFixture()
	target.inspectFn = func(name string, reveal bool) (*entities.SecretInfoReport, error) {
		return secretFixture("b-1", "db-password", time.Time{}, ""), nil
	}
	target.createFn = func(name, value string) (*entities.SecretCreateReport, error) {
		panic("a colliding transfer must not create on the target")
	}
	handler := newMoveTestHandler(moveSourceFixture(), target, idleUsageFixture())

	w := secretMoveRequest(t, handler, `{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusConflict, w.Code)

	var errResp secretMoveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "SECRET_ALREADY_EXISTS", errResp.Error.Code)
}

// TestSecretHandler_Move_PartialRidesTheEnvelope pins the partial
// contract through the HTTP surface: a failed source removal after a
// successful target install answers 200 with status "partial" and the
// failure's detail in error — the secret exists on both machines.
func TestSecretHandler_Move_PartialRidesTheEnvelope(t *testing.T) {
	source := moveSourceFixture()
	source.removeFn = func(name string) error {
		return errors.New("secret \"db-password\" is in use by container 3f2e1d0c")
	}
	handler := newMoveTestHandler(source, emptyTargetFixture(), idleUsageFixture())

	w := secretMoveRequest(t, handler, `{"targetMachineId":"machine-b","mode":"move"}`)

	require.Equal(t, http.StatusOK, w.Code)

	var got SecretMoveResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "partial", got.Status)
	assert.Contains(t, got.Error, "in use by container")
}

// TestSecretHandler_Move_BelowAdminSourceIs403 pins the admin-on-source
// gate through the HTTP surface: a write-level caller passes the route's
// write gate but the service's reveal gate refuses 403 before any engine
// is asked for the value.
func TestSecretHandler_Move_BelowAdminSourceIs403(t *testing.T) {
	// The engines stay unarmed: any engine call panics the test.
	service := newMoveTestService(&fakeSecretEngine{}, &fakeSecretEngine{}, &fakeUsageEngine{},
		&fakeAccessRepo{getPermissionFn: func(machineID, userID string) (string, error) {
			return "write", nil
		}})
	handler := NewSecretHandler(service)

	w := secretMoveRequest(t, handler, `{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusForbidden, w.Code)

	var errResp secretMoveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}
