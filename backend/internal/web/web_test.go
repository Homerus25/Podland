package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// HandleUser pins — the user-scoped twin for JSON endpoints outside the
// machine subtree (auth, users, machines Create/List). Same error
// mapping and success write as Handle, but the user ID is passed through
// as-is: empty is a legal state (the auth surface answers anonymous
// callers), so there is no identity invariant and the closure always
// runs.

// serveUser runs a user-scoped endpoint the way the router does:
// request-scoped context values, no chi URL params.
func serveUser(t *testing.T, handler http.HandlerFunc, userID string) *httptest.ResponseRecorder {
	t.Helper()

	ctx := contextutils.SetRequestID(context.Background(), "test-request-id")
	if userID != "" {
		ctx = contextutils.SetUserID(ctx, userID)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil).WithContext(ctx)

	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleUser_PassesUserIDThrough(t *testing.T) {
	var gotUserID string

	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]any, error) {
			gotUserID = userID
			return map[string]any{"authenticated": true}, nil
		})
	}, "test-user")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test-user", gotUserID, "closure must receive the context user ID")

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["authenticated"])
}

func TestHandleUser_EmptyUserIDStillRunsTheClosure(t *testing.T) {
	ran := false

	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]any, error) {
			ran = true
			assert.Empty(t, userID, "an empty user ID must be passed through, not gated")
			return map[string]any{"authenticated": false}, nil
		})
	}, "")

	assert.Equal(t, http.StatusOK, w.Code,
		"an empty user ID is a legal state (anonymous callers), not an invariant violation")

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["authenticated"])
	assert.True(t, ran, "the closure must run even without a user ID")
}

func TestHandleUser_NoContentSkipsEncoding(t *testing.T) {
	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusNoContent, func(ctx context.Context, userID string) (struct{}, error) {
			return struct{}{}, nil
		})
	}, "test-user")

	assert.Equal(t, http.StatusNoContent, w.Code, "204-skip behaviour must be mirrored from Handle")
	assert.Empty(t, w.Body.String(), "204 responses must have an empty body")
}

func TestHandleUser_AppErrorMapsToItsStatusAndCode(t *testing.T) {
	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]string, error) {
			return nil, appErrors.ErrRefreshInvalid
		})
	}, "")

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "AUTH_REFRESH_INVALID", env.Error.Code)
	assert.Equal(t, "test-request-id", env.RequestID, "envelope must carry requestId")
}

func TestHandleUser_WrappedAppErrorIsUnwrapped(t *testing.T) {
	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]string, error) {
			return nil, fmt.Errorf("refreshing session: %w", appErrors.ErrSessionExpired)
		})
	}, "")

	assert.Equal(t, http.StatusUnauthorized, w.Code, "wrapped AppErrors must keep their status")

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "AUTH_SESSION_EXPIRED", env.Error.Code)
}

func TestHandleUser_PlainErrorFallsBackTo500WithGenericMessage(t *testing.T) {
	w := serveUser(t, func(w http.ResponseWriter, r *http.Request) {
		HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (map[string]string, error) {
			return nil, errors.New("boom with secret detail")
		})
	}, "test-user")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	assert.Equal(t, "An unexpected error occurred", env.Error.Message, "5xx messages must be rewritten generic")
	assert.NotContains(t, w.Body.String(), "secret detail", "internal detail must not leak")
	assert.Equal(t, "test-request-id", env.RequestID)
}

// Decode pins — the decode twin every body-bearing handler crosses. The
// ritual (JSON decode + struct validation, one error vocabulary) is
// pinned here once, not per domain.

// decodeTarget is the tagged shape the failure cases decode into.
type decodeTarget struct {
	Name string `json:"name" validate:"required,min=1"`
}

func decodeRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/machines/m/volumes", strings.NewReader(body))
}

func TestDecode_MalformedBodyAnswersInvalidRequest(t *testing.T) {
	appErr := Decode(decodeRequest(`{not json`), &decodeTarget{})

	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Equal(t, "Invalid request body", appErr.Message)
}

func TestDecode_WrongTypedFieldAnswersInvalidRequest(t *testing.T) {
	appErr := Decode(decodeRequest(`{"name": 42}`), &decodeTarget{})

	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Equal(t, "Invalid request body", appErr.Message)
}

func TestDecode_MissingRequiredFieldAnswersValidationFailed(t *testing.T) {
	appErr := Decode(decodeRequest(`{}`), &decodeTarget{})

	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Equal(t,
		"Key: 'decodeTarget.Name' Error:Field validation for 'Name' failed on the 'required' tag",
		appErr.Message,
		"the validation failure must carry the validator's message")
}

func TestDecode_TaglessStructSkipsValidation(t *testing.T) {
	var tagless struct {
		Name string `json:"name"`
	}

	assert.Nil(t, Decode(decodeRequest(`{"name": ""}`), &tagless),
		"a DTO without validate tags imposes no rules in Decode")
}

func TestDecode_CleanBodyAnswersNil(t *testing.T) {
	var target decodeTarget

	assert.Nil(t, Decode(decodeRequest(`{"name": "my-volume", "extra": "ignored"}`), &target),
		"a clean body decodes nil, and unknown fields are ignored (no DisallowUnknownFields)")
	assert.Equal(t, "my-volume", target.Name)
}

// URLParam / QueryString / QueryBool / QueryInt pins — the param ritual
// every route-param guard and query parse crosses, pinned here once, not
// per domain.

// paramRequest builds a request carrying chi URL params the way the
// router would.
func paramRequest(params map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/machines/m/volumes", nil)
	rctx := chi.NewRouteContext()
	for name, value := range params {
		rctx.URLParams.Add(name, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestURLParam_PresentValuePassesThrough(t *testing.T) {
	value, appErr := URLParam(paramRequest(map[string]string{"volumeName": "my-volume"}), "volumeName", "MISSING_VOLUME_NAME")

	assert.Nil(t, appErr)
	assert.Equal(t, "my-volume", value)
}

func TestURLParam_EmptyValueAnswersGuardWithDerivedMessage(t *testing.T) {
	for _, tc := range []struct{ code, wantMessage string }{
		{"MISSING_VOLUME_NAME", "Volume name is required"},
		{"MISSING_CONTAINER_ID", "Container ID is required"},
		{"MISSING_QUADLET_ID", "Quadlet ID is required"},
		{"MISSING_SECRET_ID", "Secret ID is required"},
		{"MISSING_IMAGE_ID", "Image ID is required"},
		{"MISSING_USER_ID", "User ID is required"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			value, appErr := URLParam(paramRequest(nil), "volumeName", tc.code)

			require.NotNil(t, appErr)
			assert.Empty(t, value)
			assert.Equal(t, http.StatusBadRequest, appErr.Status)
			assert.Equal(t, tc.code, appErr.Code)
			assert.Equal(t, tc.wantMessage, appErr.Message,
				"the message must be derived from the code")
		})
	}
}

func TestQueryBool_ParsedValueWinsOverDefault(t *testing.T) {
	assert.True(t, QueryBool(queryRequest("force=true"), "force", false))
	assert.False(t, QueryBool(queryRequest("force=false"), "force", true))
}

func TestQueryBool_MissingOrMalformedYieldsDefault(t *testing.T) {
	assert.False(t, QueryBool(queryRequest(""), "force", false), "missing flag yields the false default")
	assert.True(t, QueryBool(queryRequest(""), "stop", true), "missing flag yields the true default")
	assert.False(t, QueryBool(queryRequest("force=yes"), "force", false), "malformed flag yields the false default")
	assert.True(t, QueryBool(queryRequest("stop=notabool"), "stop", true), "malformed flag yields the true default")
}

func TestQueryInt_ParsedValueWinsOverDefault(t *testing.T) {
	assert.Equal(t, 5, QueryInt(queryRequest("last=5"), "last", 0))
	assert.Equal(t, 7, QueryInt(queryRequest("last=NaN"), "last", 7), "malformed value yields the default")
	assert.Equal(t, 0, QueryInt(queryRequest(""), "last", 0), "missing value yields the default")
}

func TestQueryString_PresentValuePassesThrough(t *testing.T) {
	value, appErr := QueryString(queryRequest("targetMachineId=machine-b"), "targetMachineId", "MISSING_TARGET_MACHINE_ID")

	assert.Nil(t, appErr)
	assert.Equal(t, "machine-b", value)
}

func TestQueryString_EmptyValueAnswersGuardWithDerivedMessage(t *testing.T) {
	value, appErr := QueryString(queryRequest(""), "targetMachineId", "MISSING_TARGET_MACHINE_ID")

	require.NotNil(t, appErr)
	assert.Empty(t, value)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "MISSING_TARGET_MACHINE_ID", appErr.Code)
	assert.Equal(t, "Target machine ID is required", appErr.Message,
		"the message must be derived from the code")
}

func queryRequest(query string) *http.Request {
	target := "/api/machines/m/volumes"
	if query != "" {
		target += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, target, nil)
}
