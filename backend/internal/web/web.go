// Package web owns the endpoint envelope: the uniform response contract
// every JSON endpoint speaks. Failures surface as the
// {requestId, error{code, message}} envelope; successes are wire-shaped —
// a DTO with an explicit status. Three entry points share one private
// core for the error-mapping and success-write tail: Handle for
// machine-scoped JSON endpoints (behind the /api/machines/{id} subtree
// middleware), HandleStream for machine-scoped endpoints whose success
// is a byte stream or multipart upload, and HandleUser for user-scoped
// JSON endpoints outside the subtree (auth, users, machines
// Create/List), where the user ID is passed through as-is — empty is a
// legal state there, and there is no machine ID. Handlers cross this
// seam instead of re-assembling the identity guards and error mapping
// per endpoint.
//
// CONTEXT.md "endpoint envelope" is the glossary entry for this contract.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// Handle runs a machine-scoped JSON endpoint through the endpoint
// envelope. It extracts the caller and machine identities the subtree
// middleware (RequireMachineAccess / RequireMachinePermission in
// internal/middleware) has already validated, invokes call, and writes
// the result with successStatus — or the error envelope when call
// fails.
//
// Within call, everything route-specific stays plain Go: body decode
// (cross web.Decode), extra URL params (e.g. volumeName), query parsing.
// Validation failures are returned as *appErrors.AppError values (the
// sentinels from internal/errors, or NewAppError for route-specific
// codes) and ride the same envelope. Both errors.As and direct returns
// work: wrapped AppErrors are unwrapped, so services may
// fmt.Errorf("%w", …) a sentinel while adding context for logs.
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
	respond(w, r, successStatus, result, err)
}

// HandleStream runs a machine-scoped endpoint whose success is a byte
// stream or multipart upload rather than a JSON DTO through the
// endpoint envelope's error side: the same identity extraction, AppError
// mapping, and 500 fallback Handle owns, without the JSON encode. It
// extracts the caller and machine identities the subtree middleware
// (RequireMachineAccess / RequireMachinePermission in
// internal/middleware) has already validated and invokes stream; the
// closure owns everything wire-specific — response headers, the body
// ritual, the service call, and the success write (an implicit 200, an
// explicit 204, whatever the format needs).
//
// Within stream, everything route-specific stays plain Go: query
// parsing, extra URL params (e.g. volumeName). Validation failures are
// returned as *appErrors.AppError values (the sentinels from
// internal/errors, or NewAppError for route-specific codes) and ride
// the same envelope. Both errors.As and direct returns work: wrapped
// AppErrors are unwrapped via errors.AsType, so services may
// fmt.Errorf("%w", …) a sentinel while adding context for logs. Any
// other error answers the 500 fallback with Handle's log shape
// (requestID + error).
//
// A missing userID or machineID maps to 500 INTERNAL_ERROR and a loud
// log line: on the /api/machines/{id} subtree the middleware guarantees
// both, so a miss means a miswired route, not a client error — and
// stream never runs. Mounting a HandleStream endpoint outside the
// subtree is a wiring bug the log will surface.
func HandleStream(w http.ResponseWriter, r *http.Request, stream func(ctx context.Context, userID, machineID string) error) {
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

	if err := stream(r.Context(), userID, machineID); err != nil {
		respondError(w, r, err)
	}
}

// HandleUser runs a user-scoped JSON endpoint outside the machine
// subtree (auth Login/Refresh/Logout/Status, users Create, machines
// Create/List) through the endpoint envelope. It passes the caller's
// user ID from the request context to call as-is — empty included — and
// writes the result with successStatus, or the error envelope when call
// fails.
//
// Unlike Handle and HandleStream there is no identity invariant: an
// empty user ID is a legal state, because the auth surface answers
// anonymous callers (Status) and the refresh/logout paths authenticate
// by cookie, not JWT. Routes that need a caller rely on the JWT
// middleware upstream; a miswire surfaces through the envelope's
// unexpected-error log when the closure's service call fails. There is
// no machine ID for the same reason — these endpoints sit outside the
// /api/machines/{id} subtree.
//
// Within call, everything route-specific stays plain Go: body decode
// (cross web.Decode), cookie reading, and header writes — cookies are
// set inside the closure by capturing the writer; the envelope owns the
// status line and body. Validation failures are returned as
// *appErrors.AppError values (the sentinels from internal/errors, or
// NewAppError for route-specific codes) and ride the same envelope;
// wrapped AppErrors are unwrapped, and anything else answers the same
// logged 500 fallback the other entry points use.
//
// successStatus of http.StatusNoContent skips JSON encoding (the
// closure should return a zero value); any other status encodes the
// returned value as JSON.
func HandleUser[T any](w http.ResponseWriter, r *http.Request, successStatus int, call func(ctx context.Context, userID string) (T, error)) {
	userID := contextutils.GetUserIDFromContext(r.Context())

	result, err := call(r.Context(), userID)
	respond(w, r, successStatus, result, err)
}

// respond is the envelope's shared success/error tail for the JSON
// entry points (Handle, HandleUser): an error maps through respondError,
// a success writes successStatus — 204 skips encoding — with the result
// as JSON.
func respond[T any](w http.ResponseWriter, r *http.Request, successStatus int, result T, err error) {
	if err != nil {
		respondError(w, r, err)
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

// respondError is the envelope's shared error tail: a *AppError —
// wrapped included, unwrapped via errors.AsType — answers with its own
// status, code, and message; any other error logs once (requestID +
// error) and answers the generic 500 fallback so internal detail never
// leaks to the wire.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := contextutils.GetRequestID(r.Context())

	if appErr, ok := errors.AsType[*appErrors.AppError](err); ok {
		appErrors.RespondWithError(w, appErr, requestID)
		return
	}
	slog.ErrorContext(r.Context(), "endpoint envelope: unexpected error",
		slog.Any("requestID", requestID),
		slog.Any("error", err))
	appErrors.RespondWithError(w, appErrors.ErrInternalError, requestID)
}

// validate is the package-level shared validator instance behind Decode.
// validator instances are thread-safe, so one instance serves every
// request; caching struct metadata across calls is the point of sharing
// it instead of minting one per decode.
var validate = validator.New()

// Decode reads the request body into target as JSON and validates it —
// the decode+validate ritual every body-bearing handler crosses, with
// one error vocabulary. A body that is not valid JSON (syntax or
// wrong-typed field) answers 400 INVALID_REQUEST "Invalid request body";
// a body that decodes but fails target's validate tags answers 400
// VALIDATION_FAILED carrying the validator's message; a clean body
// answers nil and leaves target populated. Unknown fields are ignored
// (no DisallowUnknownFields), and a DTO without validate tags imposes no
// rules — its domain rules stay in the service. Handlers return the
// *AppError from their envelope closure (or respond it directly before a
// stream begins, as images Pull does); the envelope maps it to the wire.
func Decode(r *http.Request, target any) *appErrors.AppError {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return appErrors.ErrInvalidRequest.WithMessage("Invalid request body")
	}
	if err := validate.Struct(target); err != nil {
		return appErrors.ErrValidationFailed.WithMessage(err.Error())
	}
	return nil
}

// SetRateLimitHeaders writes the X-RateLimit-* trio — the budget's
// configured limit, the key's remaining budget, and the seconds until
// its window resets — with one owner for the wire format. The rate-limit
// middlewares advertise it on every response they pass; the
// failure-budget handlers on their 429 answers (remaining 0). Headers
// only: the status line and body stay with the caller.
func SetRateLimitHeaders(w http.ResponseWriter, limit, remaining int, resetIn time.Duration) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(resetIn.Seconds()), 10))
}
