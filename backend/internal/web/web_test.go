package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envelope is the shared response shape asserted throughout this suite.
type envelope struct {
	RequestID string `json:"requestId"`
	Error     struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// serve runs a Handle endpoint the way the router does: request-scoped
// context values plus chi URL params.
func serve(t *testing.T, handler http.HandlerFunc, userID, machineID string) *httptest.ResponseRecorder {
	t.Helper()

	ctx := contextutils.SetRequestID(context.Background(), "test-request-id")
	if userID != "" {
		ctx = contextutils.SetUserID(ctx, userID)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/machines/"+machineID+"/volumes", nil).WithContext(ctx)

	rctx := chi.NewRouteContext()
	if machineID != "" {
		rctx.URLParams.Add("id", machineID)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandle_SuccessEncodesDTOWithStatus(t *testing.T) {
	var gotUserID, gotMachineID string

	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			gotUserID, gotMachineID = userID, machineID
			return map[string]string{"name": "my-volume"}, nil
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "test-user", gotUserID, "closure must receive the context user ID")
	assert.Equal(t, "test-machine", gotMachineID, "closure must receive the chi machine ID")

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "my-volume", body["name"])
}

func TestHandle_CustomSuccessStatus(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			return map[string]string{"created": "true"}, nil
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHandle_NoContentSkipsEncoding(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
			return struct{}{}, nil
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String(), "204 responses must have an empty body")
}

func TestHandle_AppErrorMapsToItsStatusAndCode(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			return nil, appErrors.ErrVolumeNotFound
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "VOLUME_NOT_FOUND", env.Error.Code)
	assert.Equal(t, "test-request-id", env.RequestID, "envelope must carry requestId")
}

func TestHandle_WrappedAppErrorIsUnwrapped(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			return nil, fmt.Errorf("listing volumes: %w", appErrors.ErrVolumeInUse)
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusConflict, w.Code, "wrapped AppErrors must keep their status")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "VOLUME_IN_USE", env.Error.Code)
}

func TestHandle_PlainErrorFallsBackTo500WithGenericMessage(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			return nil, errors.New("boom with secret detail")
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	assert.Equal(t, "An unexpected error occurred", env.Error.Message, "5xx messages must be rewritten generic")
	assert.NotContains(t, w.Body.String(), "secret detail", "internal detail must not leak")
	assert.Equal(t, "test-request-id", env.RequestID)
}

func TestHandle_MissingUserIDIsAnInvariantViolation(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			t.Fatal("closure must not run when the identity invariant is violated")
			return nil, nil
		})
	}, "", "test-machine")

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"the subtree middleware guarantees a user ID; a miss is a miswired route, not a client error")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
}

func TestHandle_MissingMachineIDIsAnInvariantViolation(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (map[string]string, error) {
			t.Fatal("closure must not run when the identity invariant is violated")
			return nil, nil
		})
	}, "test-user", "")

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"the subtree middleware guarantees a machine ID; a miss is a miswired route, not a client error")
}

// HandleStream pins — the binary twin carries the envelope's error side
// for endpoints whose success is a byte stream or multipart upload. Its
// six behaviors are pinned here once, exactly like Handle's above.

func TestHandleStream_SuccessPassesThroughUnmodified(t *testing.T) {
	var gotUserID, gotMachineID string

	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			gotUserID, gotMachineID = userID, machineID
			w.Header().Set("Content-Type", "application/x-tar")
			w.Header().Set("Content-Disposition", `attachment; filename="my-volume.tar"`)
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("tar-bytes"))
			return err
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusOK, w.Code, "the closure's status must not be overridden")
	assert.Equal(t, "application/x-tar", w.Header().Get("Content-Type"),
		"the closure's headers must reach the response unmodified (no JSON content type)")
	assert.Equal(t, `attachment; filename="my-volume.tar"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "tar-bytes", w.Body.String(),
		"the closure's bytes must reach the response unmodified (no JSON encode)")
	assert.Equal(t, "test-user", gotUserID, "closure must receive the context user ID")
	assert.Equal(t, "test-machine", gotMachineID, "closure must receive the chi machine ID")
}

func TestHandleStream_AppErrorMapsToItsStatusAndCode(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			return appErrors.ErrVolumeNotFound
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "VOLUME_NOT_FOUND", env.Error.Code)
	assert.Equal(t, "test-request-id", env.RequestID, "envelope must carry requestId")
}

func TestHandleStream_WrappedAppErrorIsUnwrapped(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			return fmt.Errorf("exporting volume: %w", appErrors.ErrVolumeNotFound)
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusNotFound, w.Code, "wrapped AppErrors must keep their status")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "VOLUME_NOT_FOUND", env.Error.Code)
}

func TestHandleStream_PlainErrorFallsBackTo500WithGenericMessage(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			return errors.New("boom with secret detail")
		})
	}, "test-user", "test-machine")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	assert.Equal(t, "An unexpected error occurred", env.Error.Message, "5xx messages must be rewritten generic")
	assert.NotContains(t, w.Body.String(), "secret detail", "internal detail must not leak")
	assert.Equal(t, "test-request-id", env.RequestID)
}

func TestHandleStream_MissingUserIDIsAnInvariantViolation(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			t.Fatal("closure must not run when the identity invariant is violated")
			return nil
		})
	}, "", "test-machine")

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"the subtree middleware guarantees a user ID; a miss is a miswired route, not a client error")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
}

func TestHandleStream_MissingMachineIDIsAnInvariantViolation(t *testing.T) {
	w := serve(t, func(w http.ResponseWriter, r *http.Request) {
		HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
			t.Fatal("closure must not run when the identity invariant is violated")
			return nil
		})
	}, "test-user", "")

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"the subtree middleware guarantees a machine ID; a miss is a miswired route, not a client error")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
}
