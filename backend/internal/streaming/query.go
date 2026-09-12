package streaming

import (
	"net/http"
	"strconv"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// ParseFollow reads the follow flag of the log-request query language: a
// present, bool-parseable "true" selects the streaming envelope. It is
// lenient by contract: a missing or malformed value simply means false,
// never an error.
func ParseFollow(r *http.Request) bool {
	follow := false
	if followParam := r.URL.Query().Get("follow"); followParam != "" {
		if parsed, err := strconv.ParseBool(followParam); err == nil {
			follow = parsed
		}
	}
	return follow
}

// ParseLogWindow reads the strict since/tail log window: a malformed
// parameter is a 400 AppError the caller answers through the endpoint
// envelope, the defaults are the zero time and tail -1 (all lines).
func ParseLogWindow(r *http.Request) (time.Time, int64, *appErrors.AppError) {
	since := time.Time{}
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		parsedSince, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			return time.Time{}, -1, appErrors.NewAppError("INVALID_SINCE_PARAM", "Invalid 'since' parameter format, use RFC3339", http.StatusBadRequest)
		}
		since = parsedSince
	}

	tail := int64(-1)
	if tailParam := r.URL.Query().Get("tail"); tailParam != "" {
		parsedTail, err := strconv.ParseInt(tailParam, 10, 64)
		if err != nil {
			return time.Time{}, -1, appErrors.NewAppError("INVALID_TAIL_PARAM", "Invalid 'tail' parameter, use RFC3339", http.StatusBadRequest)
		}
		tail = parsedTail
	}

	return since, tail, nil
}
