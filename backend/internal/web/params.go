package web

import (
	"net/http"
	"strconv"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/go-chi/chi/v5"
)

// URLParam reads a route parameter and owns the missing-param guard: an
// empty value answers 400 with exactly the given error code and a
// message derived from it ("MISSING_VOLUME_NAME" becomes "Volume name
// is required"), so handlers shrink to one line instead of hand-minting
// the NewAppError ritual. The code stays explicit at the call site —
// the vocabulary stays domain-visible, like the podmanerr sentinels.
func URLParam(r *http.Request, name, code string) (string, *appErrors.AppError) {
	value := chi.URLParam(r, name)
	if value == "" {
		return "", appErrors.NewAppError(code, requiredMessage(code), http.StatusBadRequest)
	}
	return value, nil
}

// requiredMessage derives the missing-param message from its code:
// strip the MISSING_ prefix, split on _, capitalize the first word,
// keep ID uppercase, lowercase the rest, and append " is required".
func requiredMessage(code string) string {
	words := strings.Split(strings.TrimPrefix(code, "MISSING_"), "_")
	for i, word := range words {
		switch {
		case word == "ID":
			words[i] = "ID"
		case i == 0 && word != "":
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		default:
			words[i] = strings.ToLower(word)
		}
	}
	return strings.Join(words, " ") + " is required"
}

// QueryString reads a required string query parameter with the same
// missing-param contract as URLParam: an empty value answers 400 with
// exactly the given error code and a message derived from it. The code
// stays explicit at the call site. Optional string query parameters
// keep using r.URL.Query().Get directly (the lenient default is the
// caller's empty string).
func QueryString(r *http.Request, name, code string) (string, *appErrors.AppError) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return "", appErrors.NewAppError(code, requiredMessage(code), http.StatusBadRequest)
	}
	return value, nil
}

// QueryBool reads a boolean query flag with one lenient contract: a
// missing or malformed value yields def. The default is explicit
// because the flags disagree (delete force defaults false, quadlet
// delete stop defaults true) — a false-only helper would leave exactly
// one hand-rolled survivor, which is the drift this extraction kills.
func QueryBool(r *http.Request, name string, def bool) bool {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return parsed
}

// QueryInt reads an integer query parameter with the same lenient
// contract as QueryBool: a missing or malformed value yields def.
// Syntax leniency is the shared contract; semantics (e.g. positivity)
// stay caller-side.
func QueryInt(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return parsed
}
