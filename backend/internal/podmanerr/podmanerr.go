// Package podmanerr owns podman's error model for the backend: one adapter
// that classifies podman failures so domains don't each re-implement the
// digging. Podman's tunnel layer destroys type information on several paths
// (inspect 404s arrive as plain fmt.Errorf strings, per-item errors as
// response strings), so classification is typed-first — an
// errorhandling.ErrorModel's ResponseCode — with a phrase fallback over the
// lowercased message text. Every phrase here is matched in podman's real
// output today; nothing is invented.
//
// The predicates are independent facts, not a partition: one error can
// satisfy several (a 409 saying "already exists" is both IsConflict and
// IsAlreadyExists). Call sites check most-specific first.
//
// This package imports the stdlib and podman's errorhandling only — never
// internal/errors. AppError sentinels stay domain-owned and are mapped at
// domain call sites, which the import graph enforces.
package podmanerr

import (
	"errors"
	"net/http"
	"strings"

	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// IsNotFound reports whether err is podman's not-found answer: a 404
// ErrorModel, or the "no such X" / "not found" texts podman's tunnel layer
// and per-item reports use when the type is lost.
func IsNotFound(err error) bool {
	if code, ok := responseCode(err); ok && code == http.StatusNotFound {
		return true
	}
	return containsAny(err, "no such", "not found")
}

// IsAlreadyExists reports whether err is podman's name-collision answer:
// the "already exists" phrase, or an ErrorModel whose cause carries the
// common package's name-in-use sentinel (podman reports secret duplicates
// that way, with no distinct status).
func IsAlreadyExists(err error) bool {
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) && strings.Contains(strings.ToLower(errModel.Because), "name in use") {
		return true
	}
	return containsAny(err, "already exists")
}

// IsUniqueConstraintViolation reports whether err is podman's sqlite
// unique-constraint failure — the shape podman's rename answers a name
// collision with: a 500 with no typed conflict of its own
// (RenameContainer → RewriteContainerConfig in podman's source), so the
// constraint phrase is the collision's only signature.
func IsUniqueConstraintViolation(err error) bool {
	return containsAny(err, "unique constraint failed")
}

// IsConflict reports whether err is podman's conflict answer: a 409
// ErrorModel, or the refusal texts podman emits when a resource is still
// referenced ("in use", "used by", "unable to delete") — which arrive as
// plain errors or 500 ErrorModels on several paths.
func IsConflict(err error) bool {
	if code, ok := responseCode(err); ok && code == http.StatusConflict {
		return true
	}
	return containsAny(err, "in use", "used by", "unable to delete")
}

// IsClientError reports whether err is a client-side rejection: a 4xx
// ErrorModel other than the 404/409 answers with their own predicates, or
// podman's validation texts ("invalid argument", "names must match") that
// arrive untyped, sometimes under a 500 status.
func IsClientError(err error) bool {
	if code, ok := responseCode(err); ok &&
		code >= http.StatusBadRequest &&
		code < http.StatusInternalServerError &&
		code != http.StatusNotFound &&
		code != http.StatusConflict {
		return true
	}
	return containsAny(err, "invalid argument", "names must match")
}

// Message digs the human-readable text out of a podman error: the
// ErrorModel's message with its cause appended when non-empty, the error's
// own text otherwise. Domains use it to carry podman's refusal text to the
// frontend inside domain sentinels.
func Message(err error) string {
	if err == nil {
		return ""
	}
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) {
		msg := strings.TrimSpace(errModel.Message)
		cause := strings.TrimSpace(errModel.Because)
		switch {
		case msg != "" && cause != "":
			return msg + " " + cause
		case msg != "":
			return msg
		case cause != "":
			return cause
		}
	}
	return err.Error()
}

// responseCode digs the ErrorModel's HTTP status out of a podman error,
// reporting whether one is present anywhere in the wrap chain.
func responseCode(err error) (int, bool) {
	var errModel *errorhandling.ErrorModel
	if !errors.As(err, &errModel) {
		return 0, false
	}
	return errModel.ResponseCode, true
}

// containsAny reports whether any phrase appears in the error's combined
// text: the dug message and cause, plus the full error string (wrap
// prefixes included), lowercased. The combination covers every shape podman
// errors take on the backend — plain tunnel strings, per-item report
// errors, and wrapped ErrorModels whose Message alone is only part of the
// story.
func containsAny(err error, phrases ...string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(Message(err) + " " + err.Error())
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
