package volumes

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// moveErrorEnvelope mirrors the {"requestId","error":{code,message}}
// shape the endpoint envelope answers failures with — only the fields the
// assertions consume are named.
type moveErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// newMoveTestHandler builds the handler over the real service with
// per-machine fake engines, a source-machine usage fake, and a
// write-allowing access store — the wiring the move endpoints exercise.
func newMoveTestHandler(sourceEngine, targetEngine *fakeVolumeEngine, usage *fakeUsageEngine) *VolumeHandler {
	service := newMoveTestService(sourceEngine, targetEngine, usage,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})
	return NewVolumeHandler(service)
}

func TestVolumeHandler_MovePreview_ReturnsPreview(t *testing.T) {
	usage := &fakeUsageEngine{
		containerMountsFn: func() ([]ContainerMounts, error) {
			return []ContainerMounts{{Name: "web", Mounts: []string{"pgdata:/var/lib/pg:Z"}}}, nil
		},
		listQuadletNamesFn: func() ([]string, error) {
			return []string{"web.container"}, nil
		},
		printQuadletFn: func(string) (string, error) {
			return "[Container]\nImage=busybox\nVolume=pgdata:/var/lib/pg:Z\n", nil
		},
	}

	w := testutils.Perform(newMoveTestHandler(moveSourceFixture(t), emptyTargetFixture(), usage).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes/pgdata/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "pgdata"},
	})

	require.Equal(t, http.StatusOK, w.Code)

	var got VolumeMovePreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.InUse, "a container mounting the volume marks it in use")
	assert.False(t, got.Collision, "an empty target holds no same-named volume")
	require.Len(t, got.ReferencingQuadlets, 1)
	assert.Equal(t, "web.container", got.ReferencingQuadlets[0].Name)
}

func TestVolumeHandler_MovePreview_EmptyQuadletsRendersAsArray(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(moveSourceFixture(t), emptyTargetFixture(), idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes/pgdata/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "pgdata"},
	})

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"collision":false,"inUse":false,"referencingQuadlets":[]}`, w.Body.String(),
		"a machine without quadlets must render an empty JSON array, not null")
}

func TestVolumeHandler_MovePreview_MissingVolumeNameIs400(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(&fakeVolumeEngine{}, &fakeVolumeEngine{}, idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes//move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": ""},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVolumeHandler_MovePreview_MissingTargetMachineIDIs400(t *testing.T) {
	w := testutils.Perform(newMoveTestHandler(&fakeVolumeEngine{}, &fakeVolumeEngine{}, idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes/pgdata/move-preview",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "pgdata"},
	})

	require.Equal(t, http.StatusBadRequest, w.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "MISSING_TARGET_MACHINE_ID", errResp.Error.Code)
}

func TestVolumeHandler_MovePreview_TargetAccessDeniedIs403(t *testing.T) {
	// The handler drives the real service, so the 403 comes from the
	// service's target gate: the access store answers with no row and the
	// source engine's inspect is armed to panic, proving the source lease
	// was never crossed (no false preview).
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			return "", nil
		}})
	handler := NewVolumeHandler(service)

	w := testutils.Perform(handler.MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes/pgdata/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "pgdata"},
	})

	require.Equal(t, http.StatusForbidden, w.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}

func TestVolumeHandler_MovePreview_MissingSourceVolumeIs404(t *testing.T) {
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) { return nil, nil },
	}

	w := testutils.Perform(newMoveTestHandler(source, emptyTargetFixture(), idleUsageFixture()).MovePreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/volumes/gone/move-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "gone"},
	})

	require.Equal(t, http.StatusNotFound, w.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "VOLUME_NOT_FOUND", errResp.Error.Code)
}

// --- The transfer handler (always NDJSON) ---

// parseMoveNDJSON decodes a streamed move response body into events.
func parseMoveNDJSON(t *testing.T, body string) []VolumeMoveEvent {
	t.Helper()
	var events []VolumeMoveEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event VolumeMoveEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must be a valid event: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// moveRequest performs a POST move through the handler with the given
// JSON body and the standard machine and volume params.
func moveRequest(handler *VolumeHandler, body string) *httptest.ResponseRecorder {
	return testutils.Perform(handler.Move, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/volumes/pgdata/move",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": "pgdata"},
		Body:      strings.NewReader(body),
	})
}

func TestVolumeHandler_Move_StreamsNDJSON(t *testing.T) {
	imported := ""
	target := emptyTargetFixture()
	target.importFn = func(name string, r io.Reader) error {
		data, err := io.ReadAll(r)
		imported = string(data)
		return err
	}
	handler := newMoveTestHandler(moveSourceFixture(t), target, idleUsageFixture())

	w := moveRequest(handler, `{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseMoveNDJSON(t, w.Body.String())
	require.Len(t, events, 3, "volume start, volume done, terminal complete")

	assert.Equal(t, VolumeMoveEventTypeVolume, events[0].Type)
	assert.Equal(t, VolumeMovePhaseStart, events[0].Phase)
	assert.Equal(t, "pgdata", events[0].Volume)
	assert.Equal(t, "machine-a", events[0].MachineID)
	assert.NotEmpty(t, events[0].OperationID, "the transport stamps its operation ID")
	assert.NotEmpty(t, events[0].Timestamp)

	assert.Equal(t, VolumeMoveEventTypeVolume, events[1].Type)
	assert.Equal(t, VolumeMovePhaseDone, events[1].Phase)

	assert.Equal(t, VolumeMoveEventTypeComplete, events[2].Type)
	require.NotNil(t, events[2].Result)
	assert.Equal(t, VolumeMoveStatusCopied, events[2].Result.Status)
	assert.Equal(t, "pgdata", events[2].Result.Name)
	assert.Equal(t, "machine-a", events[2].Result.SourceMachineID)
	assert.Equal(t, "machine-b", events[2].Result.TargetMachineID)
	assert.Greater(t, events[2].Result.Bytes, int64(0))
	assert.NotEmpty(t, imported, "the data crossed the splice")

	for _, event := range events {
		assert.Equal(t, events[0].OperationID, event.OperationID)
		assert.Equal(t, "machine-a", event.MachineID)
	}
}

func TestVolumeHandler_Move_MoveModeCompleteEventIsMoved(t *testing.T) {
	handler := newMoveTestHandler(movableSourceFixture(t), emptyTargetFixture(), idleUsageFixture())

	w := moveRequest(handler, `{"targetMachineId":"machine-b","mode":"move"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseMoveNDJSON(t, w.Body.String())
	require.NotEmpty(t, events)
	terminal := events[len(events)-1]
	require.Equal(t, VolumeMoveEventTypeComplete, terminal.Type)
	require.NotNil(t, terminal.Result)
	assert.Equal(t, VolumeMoveStatusMoved, terminal.Result.Status)
}

func TestVolumeHandler_Move_MissingVolumeNameIs400(t *testing.T) {
	handler := newMoveTestHandler(&fakeVolumeEngine{}, &fakeVolumeEngine{}, idleUsageFixture())

	w := testutils.Perform(handler.Move, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/volumes//move",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "volumeName": ""},
		Body:      strings.NewReader(`{"targetMachineId":"machine-b","mode":"copy"}`),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVolumeHandler_Move_BodyValidationIs400(t *testing.T) {
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
			handler := newMoveTestHandler(&fakeVolumeEngine{}, &fakeVolumeEngine{}, idleUsageFixture())

			w := moveRequest(handler, tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)

			var errResp moveErrorEnvelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
			assert.Equal(t, tt.want, errResp.Error.Code)
		})
	}
}

// TestVolumeHandler_Move_CollisionIsTerminalErrorEvent pins the failure
// half of the streaming contract: the collision re-check's 409 rides the
// NDJSON transport's terminal error event with the mapped code, not a
// plain envelope.
func TestVolumeHandler_Move_CollisionIsTerminalErrorEvent(t *testing.T) {
	target := emptyTargetFixture()
	target.inspectFn = func(name string) (*entities.VolumeInspectReport, error) {
		return volumeInspectFixture(name), nil
	}
	target.createFn = func(entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
		panic("a colliding transfer must not create on the target")
	}
	handler := newMoveTestHandler(moveSourceFixture(t), target, idleUsageFixture())

	w := moveRequest(handler, `{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusOK, w.Code, "the stream itself succeeds; the failure rides the terminal event")

	events := parseMoveNDJSON(t, w.Body.String())
	require.Len(t, events, 1, "the refusal fires before any progress event")
	assert.Equal(t, VolumeMoveEventTypeError, events[0].Type)
	require.NotNil(t, events[0].Error)
	assert.Equal(t, "VOLUME_ALREADY_EXISTS", events[0].Error.Code)
}

// TestVolumeHandler_Move_RollbackRidesTerminalErrorEvent pins the
// rollback's visibility: an import failure after the target creation
// answers the terminal error event with the migration-failure code and a
// message stating the rollback.
func TestVolumeHandler_Move_RollbackRidesTerminalErrorEvent(t *testing.T) {
	target := emptyTargetFixture()
	target.importFn = func(string, io.Reader) error {
		return errors.New("import refused")
	}
	target.removeFn = func(string, bool) error {
		return nil
	}
	handler := newMoveTestHandler(moveSourceFixture(t), target, idleUsageFixture())

	w := moveRequest(handler, `{"targetMachineId":"machine-b","mode":"copy"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseMoveNDJSON(t, w.Body.String())
	require.NotEmpty(t, events)
	terminal := events[len(events)-1]
	assert.Equal(t, VolumeMoveEventTypeError, terminal.Type)
	require.NotNil(t, terminal.Error)
	assert.Equal(t, "VOLUME_MIGRATION_FAILED", terminal.Error.Code)
	assert.Contains(t, terminal.Error.Message, "partially transferred target volume was removed",
		"the terminal error event states the rollback")
}

// TestVolumeHandler_Move_PartialRidesCompleteEvent pins the partial
// vocabulary on the wire: a failed source removal after a successful copy
// answers the terminal complete event with status "partial" and the
// failure's detail — the stream succeeds.
func TestVolumeHandler_Move_PartialRidesCompleteEvent(t *testing.T) {
	source := movableSourceFixture(t)
	source.removeFn = func(string, bool) error {
		return errors.New("source read-only")
	}
	handler := newMoveTestHandler(source, emptyTargetFixture(), idleUsageFixture())

	w := moveRequest(handler, `{"targetMachineId":"machine-b","mode":"move"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseMoveNDJSON(t, w.Body.String())
	require.NotEmpty(t, events)
	terminal := events[len(events)-1]
	require.Equal(t, VolumeMoveEventTypeComplete, terminal.Type)
	require.NotNil(t, terminal.Result)
	assert.Equal(t, VolumeMoveStatusPartial, terminal.Result.Status)
	assert.Contains(t, terminal.Result.Error, "source read-only")
}
