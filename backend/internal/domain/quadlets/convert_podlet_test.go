package quadlets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleCompose is the verified podlet 0.3.2 sample (spec #336 notes): a
// service, an optioned named volume (a named volume without additional
// options produces no .volume file), and a network.
const sampleCompose = `services:
  web:
    image: nginx
    volumes:
      - dbdata:/var/lib/data
    networks:
      - frontend
volumes:
  dbdata:
    driver: local
    driver_opts:
      type: tmpfs
networks:
  frontend:
    driver: bridge
`

// unsupportedOptionCompose uses both build: and image:, which podlet
// rejects (verified 0.3.2: exit 1, the error chain on stderr).
const unsupportedOptionCompose = `services:
  app:
    build: .
    image: app:latest
`

// TestConvertCompose_PodletEndToEnd runs REAL podlet through the real exec
// adapter, the real service, and the handler over HTTP (spec #336): the
// sample compose converts to the expected entries, and the
// unsupported-option compose answers 400 VALIDATION_FAILED carrying
// podlet's stderr. It needs no machine or test host; it skips when podlet
// is not on PATH.
func TestConvertCompose_PodletEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("podlet"); err != nil {
		t.Skip("podlet not on PATH")
	}

	handler := newConvertTestHandler(NewPodletComposeConverter())

	t.Run("sample compose converts to the expected entries", func(t *testing.T) {
		body, err := json.Marshal(QuadletConvertComposeRequest{Content: sampleCompose})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		handler.ConvertCompose(w, convertComposeRequest(t, string(body)))

		require.Equal(t, http.StatusOK, w.Code)

		var response QuadletConvertComposeResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))

		// Sorted by name: dbdata, frontend, web. The contents are
		// podlet 0.3.2's verified output for this compose.
		require.Len(t, response.Quadlets, 3)
		assert.Equal(t, QuadletImportEntry{
			Name:     "dbdata",
			Resource: "volume",
			Content:  "[Volume]\nDriver=local\nType=tmpfs\n",
		}, response.Quadlets[0])
		assert.Equal(t, QuadletImportEntry{
			Name:     "frontend",
			Resource: "network",
			Content:  "[Network]\nDriver=bridge\n",
		}, response.Quadlets[1])
		assert.Equal(t, QuadletImportEntry{
			Name:     "web",
			Resource: "container",
			Content:  "[Container]\nImage=nginx\nNetwork=frontend.network\nVolume=dbdata.volume:/var/lib/data\n",
		}, response.Quadlets[2])
	})

	t.Run("unsupported option answers 400 with podlet's stderr", func(t *testing.T) {
		body, err := json.Marshal(QuadletConvertComposeRequest{Content: unsupportedOptionCompose})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		handler.ConvertCompose(w, convertComposeRequest(t, string(body)))

		require.Equal(t, http.StatusBadRequest, w.Code)

		var envelope errorEnvelope
		require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
		assert.Equal(t, "VALIDATION_FAILED", envelope.Error.Code)
		assert.Contains(t, envelope.Error.Message,
			"error converting service `app`: `image` and `build` cannot both be set",
			"the message must carry podlet's own error text")
		assert.NotContains(t, envelope.Error.Message, "\x1b",
			"ANSI color escapes must be stripped from the wire message")
	})
}
