package streaming

import (
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// streamHandle is the handle begin returns: the NDJSON writer plus the
// caller and host identities the subtree middleware already validated.
type streamHandle struct {
	Writer *ndjsonWriter
	UserID string
	HostID string
}

// begin runs the entry envelope for NDJSON endpoints, mirroring the JSON
// envelope (internal/web.Handle): NDJSON headers, writer creation, and
// identity extraction in one place. It returns nil after writing the
// response itself when the endpoint cannot stream — callers just return.
//
// A missing userID or hostID maps to 500 INTERNAL_ERROR and a loud log
// line: on the /api/hosts/{id} subtree the middleware guarantees both,
// so a miss means a miswired route, not a client error (same ruling as
// web.Handle). Unsupported flushing answers 500 STREAMING_NOT_SUPPORTED.
func begin(w http.ResponseWriter, r *http.Request) *streamHandle {
	requestID := contextutils.GetRequestID(r.Context())
	userID := contextutils.GetUserIDFromContext(r.Context())
	hostID := r.PathValue("id")

	if userID == "" || hostID == "" {
		slog.ErrorContext(r.Context(), "streaming envelope: identity invariant violated (route mounted outside the host subtree?)",
			slog.Any("requestID", requestID),
			slog.Any("userID", userID),
			slog.Any("hostID", hostID))
		appErrors.RespondWithError(w, appErrors.ErrInternalError, requestID)
		return nil
	}

	writer, err := newNDJSONWriter(w)
	if err != nil {
		appErrors.RespondWithHTTPError(w, http.StatusInternalServerError, "STREAMING_NOT_SUPPORTED", "Streaming not supported", requestID)
		slog.ErrorContext(r.Context(), "Streaming not supported")
		return nil
	}

	setNDJSONHeaders(w)
	return &streamHandle{Writer: writer, UserID: userID, HostID: hostID}
}
