package streaming

import (
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
)

// Stream is the handle Begin returns: the NDJSON writer plus the caller
// and machine identities the subtree middleware already validated.
type Stream struct {
	Writer    *NDJSONWriter
	UserID    string
	MachineID string
}

// Begin runs the entry envelope for NDJSON endpoints, mirroring the JSON
// envelope (internal/web.Handle): NDJSON headers, writer creation, and
// identity extraction in one place. It returns nil after writing the
// response itself when the endpoint cannot stream — callers just return.
//
// A missing userID or machineID maps to 500 INTERNAL_ERROR and a loud log
// line: on the /api/machines/{id} subtree the middleware guarantees both,
// so a miss means a miswired route, not a client error (same ruling as
// web.Handle). Unsupported flushing answers 500 STREAMING_NOT_SUPPORTED.
func Begin(w http.ResponseWriter, r *http.Request) *Stream {
	requestID := contextutils.GetRequestID(r.Context())
	userID := contextutils.GetUserIDFromContext(r.Context())
	machineID := chi.URLParam(r, "id")

	if userID == "" || machineID == "" {
		slog.ErrorContext(r.Context(), "streaming envelope: identity invariant violated (route mounted outside the machine subtree?)",
			slog.Any("requestID", requestID),
			slog.Any("userID", userID),
			slog.Any("machineID", machineID))
		appErrors.RespondWithError(w, appErrors.ErrInternalError, requestID)
		return nil
	}

	writer, err := NewNDJSONWriter(w)
	if err != nil {
		appErrors.RespondWithHTTPError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported", requestID)
		slog.ErrorContext(r.Context(), "Streaming not supported")
		return nil
	}

	SetNDJSONHeaders(w)
	return &Stream{Writer: writer, UserID: userID, MachineID: machineID}
}
