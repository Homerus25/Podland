package images

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
)

// copyErrorEnvelope mirrors the {"requestId","error":{code,message}}
// shape the endpoint envelope answers failures with — only the fields the
// assertions consume are named.
type copyErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// newCopyTestHandler builds the handler over the real service with
// per-machine fake engines and a write-allowing access store — the wiring
// the copy endpoints exercise.
func newCopyTestHandler(source, target *fakeImageEngine) *ImageHandler {
	service := newCopyTestService(source, target, &fakeAccessRepo{getPermissionFn: allowTargetWrite})
	return NewImageHandler(service)
}

func TestImageHandler_CopyPreview_ReturnsPreview(t *testing.T) {
	w := testutils.Perform(newCopyTestHandler(copySourceFixture(), copyTargetFixture()).CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images/" + copyImageID + "/copy-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": copyImageID},
	})

	require.Equal(t, http.StatusOK, w.Code)

	var got ImageCopyPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.Present, "a target without the image answers absent")
}

func TestImageHandler_CopyPreview_PresentTargetAnswersTrue(t *testing.T) {
	target := copyTargetFixture()
	target.existsFn = func(string) (bool, error) {
		return true, nil
	}

	w := testutils.Perform(newCopyTestHandler(copySourceFixture(), target).CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images/" + copyImageID + "/copy-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": copyImageID},
	})

	require.Equal(t, http.StatusOK, w.Code)

	var got ImageCopyPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Present, "a target holding the image by ID answers present")
}

func TestImageHandler_CopyPreview_MissingImageIDIs400(t *testing.T) {
	w := testutils.Perform(newCopyTestHandler(&fakeImageEngine{}, &fakeImageEngine{}).CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images//copy-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": ""},
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImageHandler_CopyPreview_MissingTargetMachineIDIs400(t *testing.T) {
	w := testutils.Perform(newCopyTestHandler(&fakeImageEngine{}, &fakeImageEngine{}).CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images/" + copyImageID + "/copy-preview",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": copyImageID},
	})

	require.Equal(t, http.StatusBadRequest, w.Code)

	var errResp copyErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "MISSING_TARGET_MACHINE_ID", errResp.Error.Code)
}

func TestImageHandler_CopyPreview_TargetAccessDeniedIs403(t *testing.T) {
	// The handler drives the real service, so the 403 comes from the
	// service's target gate: the access store answers with no row and
	// the source engine's exists check is armed to panic, proving the
	// source lease was never crossed (no false preview).
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newCopyTestService(source, copyTargetFixture(), &fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
		return "", nil
	}})
	handler := NewImageHandler(service)

	w := testutils.Perform(handler.CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images/" + copyImageID + "/copy-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": copyImageID},
	})

	require.Equal(t, http.StatusForbidden, w.Code)

	var errResp copyErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}

func TestImageHandler_CopyPreview_MissingSourceImageIs404(t *testing.T) {
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
	}

	w := testutils.Perform(newCopyTestHandler(source, copyTargetFixture()).CopyPreview, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/machines/machine-a/images/sha256:gone/copy-preview?targetMachineId=machine-b",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": "sha256:gone"},
	})

	require.Equal(t, http.StatusNotFound, w.Code)

	var errResp copyErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "IMAGE_NOT_FOUND", errResp.Error.Code)
}

// --- The copy handler (always NDJSON) ---

// parseCopyNDJSON decodes a streamed copy response body into events.
func parseCopyNDJSON(t *testing.T, body string) []ImageCopyEvent {
	t.Helper()
	var events []ImageCopyEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event ImageCopyEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must be a valid event: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// copyRequest performs a POST copy through the handler with the given
// JSON body and the standard machine and image params.
func copyRequest(handler *ImageHandler, body string) *httptest.ResponseRecorder {
	return testutils.Perform(handler.Copy, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/images/" + copyImageID + "/copy",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": copyImageID},
		Body:      strings.NewReader(body),
	})
}

func TestImageHandler_Copy_StreamsNDJSON(t *testing.T) {
	loaded := int64(0)
	target := copyTargetFixture()
	target.loadFn = func(r io.Reader) ([]string, error) {
		data, err := io.ReadAll(r)
		loaded = int64(len(data))
		if err != nil {
			return nil, err
		}
		return []string{"registry.example/app:latest"}, nil
	}
	handler := newCopyTestHandler(copySourceFixture(), target)

	w := copyRequest(handler, `{"targetMachineId":"machine-b"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseCopyNDJSON(t, w.Body.String())
	require.GreaterOrEqual(t, len(events), 3, "image start, image done, terminal complete")

	assert.Equal(t, ImageCopyEventTypeImage, events[0].Type)
	assert.Equal(t, ImageCopyPhaseStart, events[0].Phase)
	assert.Equal(t, copyImageID, events[0].Image)
	assert.Equal(t, "machine-a", events[0].MachineID)
	assert.NotEmpty(t, events[0].OperationID, "the transport stamps its operation ID")
	assert.NotEmpty(t, events[0].Timestamp)

	done := events[len(events)-2]
	assert.Equal(t, ImageCopyEventTypeImage, done.Type)
	assert.Equal(t, ImageCopyPhaseDone, done.Phase)

	complete := events[len(events)-1]
	assert.Equal(t, ImageCopyEventTypeComplete, complete.Type)
	require.NotNil(t, complete.Result)
	assert.Equal(t, ImageCopyStatusCopied, complete.Result.Status)
	assert.Equal(t, copyImageID, complete.Result.ID)
	assert.Equal(t, "machine-a", complete.Result.SourceMachineID)
	assert.Equal(t, "machine-b", complete.Result.TargetMachineID)
	assert.Greater(t, complete.Result.Bytes, int64(0))
	assert.Equal(t, []string{"registry.example/app:latest"}, complete.Result.Names)
	assert.Equal(t, int64(len(copyArchive)), loaded, "the archive crossed the splice")

	for _, event := range events {
		assert.Equal(t, events[0].OperationID, event.OperationID)
		assert.Equal(t, "machine-a", event.MachineID)
	}
}

func TestImageHandler_Copy_MissingImageIDIs400(t *testing.T) {
	handler := newCopyTestHandler(&fakeImageEngine{}, &fakeImageEngine{})

	w := testutils.Perform(handler.Copy, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-a/images//copy",
		UserID:    "user123",
		URLParams: map[string]string{"id": "machine-a", "imageId": ""},
		Body:      strings.NewReader(`{"targetMachineId":"machine-b"}`),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestImageHandler_Copy_BodyValidationIs400(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed JSON", body: `{`, want: "INVALID_REQUEST"},
		{name: "missing targetMachineId", body: `{}`, want: "VALIDATION_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newCopyTestHandler(&fakeImageEngine{}, &fakeImageEngine{})

			w := copyRequest(handler, tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)

			var errResp copyErrorEnvelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
			assert.Equal(t, tt.want, errResp.Error.Code)
		})
	}
}

// TestImageHandler_Copy_TargetAccessDeniedIsTerminalErrorEvent pins the
// failure half of the streaming contract: the target write gate's 403
// rides the NDJSON transport's terminal error event with the mapped
// code, not a plain envelope.
func TestImageHandler_Copy_TargetAccessDeniedIsTerminalErrorEvent(t *testing.T) {
	// Read on the target is below the write the copy needs; the source
	// engine panics on any call: the gate refuses before any podman work.
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newCopyTestService(source, copyTargetFixture(), &fakeAccessRepo{getPermissionFn: allowTargetRead})
	handler := NewImageHandler(service)

	w := copyRequest(handler, `{"targetMachineId":"machine-b"}`)

	require.Equal(t, http.StatusOK, w.Code, "the stream itself succeeds; the refusal rides the terminal event")

	events := parseCopyNDJSON(t, w.Body.String())
	require.Len(t, events, 1, "the refusal fires before any progress event")
	assert.Equal(t, ImageCopyEventTypeError, events[0].Type)
	require.NotNil(t, events[0].Error)
	assert.Equal(t, "ACCESS_DENIED", events[0].Error.Code)
}

func TestImageHandler_Copy_MissingSourceImageIsTerminalErrorEvent(t *testing.T) {
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
	}
	handler := newCopyTestHandler(source, copyTargetFixture())

	w := copyRequest(handler, `{"targetMachineId":"machine-b"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseCopyNDJSON(t, w.Body.String())
	require.Len(t, events, 1, "the refusal fires before any progress event")
	assert.Equal(t, ImageCopyEventTypeError, events[0].Type)
	require.NotNil(t, events[0].Error)
	assert.Equal(t, "IMAGE_NOT_FOUND", events[0].Error.Code)
}

// TestImageHandler_Copy_BoundTripsIsTerminalErrorEvent pins the 413's
// wire shape: the bound's refusal rides the terminal error event with
// the images domain's too-large code.
func TestImageHandler_Copy_BoundTripsIsTerminalErrorEvent(t *testing.T) {
	service := newCopyTestService(copySourceFixture(), copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite}).
		WithMaxImageTransferBytes(1 << 20)
	handler := NewImageHandler(service)

	w := copyRequest(handler, `{"targetMachineId":"machine-b"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseCopyNDJSON(t, w.Body.String())
	require.NotEmpty(t, events)
	terminal := events[len(events)-1]
	assert.Equal(t, ImageCopyEventTypeError, terminal.Type)
	require.NotNil(t, terminal.Error)
	assert.Equal(t, "IMAGE_TRANSFER_TOO_LARGE", terminal.Error.Code)
}

// TestImageHandler_Copy_LoadFailureIsTerminalErrorEvent pins the
// transfer-failure vocabulary on the wire: a load that fails mid-stream
// answers the terminal error event with the transfer-failed code.
func TestImageHandler_Copy_LoadFailureIsTerminalErrorEvent(t *testing.T) {
	target := copyTargetFixture()
	target.loadFn = func(io.Reader) ([]string, error) {
		return nil, errors.New("load refused")
	}
	handler := newCopyTestHandler(copySourceFixture(), target)

	w := copyRequest(handler, `{"targetMachineId":"machine-b"}`)

	require.Equal(t, http.StatusOK, w.Code)

	events := parseCopyNDJSON(t, w.Body.String())
	require.NotEmpty(t, events)
	terminal := events[len(events)-1]
	assert.Equal(t, ImageCopyEventTypeError, terminal.Type)
	require.NotNil(t, terminal.Error)
	assert.Equal(t, "IMAGE_TRANSFER_FAILED", terminal.Error.Code)
}
