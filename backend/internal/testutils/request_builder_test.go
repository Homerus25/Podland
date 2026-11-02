package testutils

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerform_WiresContextIdentityAndURLParams(t *testing.T) {
	var gotUserID, gotRequestID, gotMachineID, gotVolumeName string

	w := Perform(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = contextutils.GetUserIDFromContext(r.Context())
		gotRequestID = contextutils.GetRequestID(r.Context())
		gotMachineID = r.PathValue("id")
		gotVolumeName = r.PathValue("volumeName")
		w.WriteHeader(http.StatusNoContent)
	}, Request{
		Method:    http.MethodDelete,
		Path:      "/api/machines/test-machine/volumes/my-volume",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-machine", "volumeName": "my-volume"},
	})

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "test-user", gotUserID)
	assert.Equal(t, "test-request-id", gotRequestID, "requestID defaults when unset")
	assert.Equal(t, "test-machine", gotMachineID)
	assert.Equal(t, "my-volume", gotVolumeName)
}

func TestPerform_BodyDefaultsToJSONContentType(t *testing.T) {
	var contentType, body string

	Perform(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		read, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body = string(read)
		w.WriteHeader(http.StatusOK)
	}, Request{
		Method: http.MethodPost,
		Path:   "/api/machines/test-machine/volumes",
		UserID: "test-user",
		Body:   strings.NewReader(`{"name":"v"}`),
	})

	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, `{"name":"v"}`, body)
}

func TestPerform_LayersIdentityOntoInjectedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotUserID string
	var gotErr error

	Perform(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = contextutils.GetUserIDFromContext(r.Context())
		gotErr = r.Context().Err()
		w.WriteHeader(http.StatusOK)
	}, Request{
		Method:  http.MethodGet,
		Path:    "/api/machines/test-machine/volumes",
		UserID:  "test-user",
		Context: ctx,
	})

	assert.Equal(t, "test-user", gotUserID, "identity must layer onto the injected context")
	assert.ErrorIs(t, gotErr, context.Canceled, "a cancelled base context must stay cancelled")
}

func TestPerform_AllowsAnonymousContext(t *testing.T) {
	var gotUserID = "sentinel"

	Perform(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = contextutils.GetUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}, Request{Method: http.MethodGet, Path: "/api/machines/m/volumes"})

	assert.Empty(t, gotUserID, "empty UserID must produce an identity-less context")
}
