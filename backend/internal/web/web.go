// Package web owns the endpoint envelope: the uniform response contract
// every machine-scoped JSON endpoint speaks. Failures surface as the
// {requestId, error{code, message}} envelope, successes as a DTO with an
// explicit status. Handlers cross this seam instead of re-assembling the
// identity guards, error mapping, and JSON encode per endpoint.
//
// CONTEXT.md "endpoint envelope" is the glossary entry for this contract.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
)

// Handle runs a machine-scoped JSON endpoint through the endpoint
// envelope. It extracts the caller and machine identities the subtree
// middleware (RequireMachineAccess / RequireMachinePermission in
// internal/middleware) has already validated, invokes call, and writes
// the result with successStatus — or the error envelope when call
// fails.
//
// Within call, everything route-specific stays plain Go: body decode,
// extra URL params (e.g. volumeName), query parsing. Validation
// failures are returned as *appErrors.AppError values (the sentinels
// from internal/errors, or NewAppError for route-specific codes) and
// ride the same envelope. Both errors.As and direct returns work:
// wrapped AppErrors are unwrapped, so services may fmt.Errorf("%w", …)
// a sentinel while adding context for logs.
//
// successStatus of http.StatusNoContent skips JSON encoding (the
// closure should return a zero value); any other status encodes the
// returned value as JSON.
//
// A missing userID or machineID maps to 500 INTERNAL_ERROR and a loud
// log line: on the /api/machines/{id} subtree the middleware guarantees
// both, so a miss means a miswired route, not a client error. Mounting
// a Handle endpoint outside the subtree is a wiring bug the log will
// surface.
func Handle[T any](w http.ResponseWriter, r *http.Request, successStatus int, call func(ctx context.Context, userID, machineID string) (T, error)) {
	requestID := contextutils.GetRequestID(r.Context())
	userID := contextutils.GetUserIDFromContext(r.Context())
	machineID := chi.URLParam(r, "id")

	if userID == "" || machineID == "" {
		slog.ErrorContext(r.Context(), "endpoint envelope: identity invariant violated (route mounted outside the machine subtree?)",
			slog.Any("requestID", requestID),
			slog.Any("userID", userID),
			slog.Any("machineID", machineID))
		appErrors.RespondWithError(w, appErrors.ErrInternalError, requestID)
		return
	}

	result, err := call(r.Context(), userID, machineID)
	if err != nil {
		if appErr, ok := errors.AsType[*appErrors.AppError](err); ok {
			appErrors.RespondWithError(w, appErr, requestID)
			return
		}
		slog.ErrorContext(r.Context(), "endpoint envelope: unexpected error",
			slog.Any("requestID", requestID),
			slog.Any("error", err))
		appErrors.RespondWithError(w, appErrors.ErrInternalError, requestID)
		return
	}

	if successStatus == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(successStatus)
	json.NewEncoder(w).Encode(result)
}
