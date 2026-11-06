// Package bulkremove owns the bulk-remove contract shared by every
// host-scoped resource's POST /{id}/<domain>/remove endpoint (issue 464,
// spec 461): the request DTO, the per-entry report DTO, and the
// per-item classification loop.
//
// The module is transport-agnostic and imports no domain vocabulary.
// Two per-item shapes are supported: the per-item loop (`Run`) calls
// the caller's existing single-item service method, passed in, so every
// entry rides the domain's own removal classification (internal/removal)
// for free and the single-item and bulk surfaces cannot drift; the
// batch-report domains (images, quadlets — the two resources whose
// podman engine offers a batch remove endpoint) build their entries from
// one batch call's report instead, classifying each report error through
// the same exported `Classify` so the classification matrix stays pinned
// once here. The domains keep only their composition pins.
//
// GLOSSARY.md "bulk-remove report" is the glossary entry for this
// contract.
package bulkremove

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// IDsRequest is the ids-only request body: the contract of a resource
// whose single-item delete has no force path at all (secrets). It is
// deliberately a separate type rather than Request with the field
// ignored, so "there is no force parameter" is a fact of the type and
// not a convention — a client cannot ask for one, and the doku offers
// none. A body that carries `force` anyway decodes with the field
// dropped.
//
// The validate tags are the whole request contract: ids is required and
// non-empty with no empty entry, and web.Decode owns the 400s
// (VALIDATION_FAILED for a tag failure, INVALID_REQUEST for a malformed
// body). Request embeds this type, so the two bodies cannot drift.
type IDsRequest struct {
	// IDs of the items to remove, in the order the report answers them.
	IDs []string `json:"ids" validate:"required,min=1,dive,required"`
}

// Request is the bulk-remove request body every resource with a force
// path shares: the ids-only contract plus the force flag. IDs carries
// whatever the resource's single-item delete addresses — an id for
// containers, images, pods and quadlets, a name for volumes and
// networks — so one field name serves them all.
type Request struct {
	IDsRequest
	// Force removes items the engine would otherwise refuse (in use,
	// running). Defaults false.
	Force bool `json:"force"`
}

// Entry is one entry in the bulk-remove report: the requested id, the
// outcome, and — on a failure — the code and message the client shows.
type Entry struct {
	// ID echoes the requested id (or name) this entry reports on.
	ID string `json:"id"`
	// Success reports whether this item was removed.
	Success bool `json:"success"`
	// Error carries the failure's code and message; absent on success.
	Error *EntryError `json:"error,omitempty"`
}

// EntryError is the code/message pair a failed entry carries. The code
// is the domain's own sentinel code (CONTAINER_RUNNING, VOLUME_IN_USE,
// …) or INTERNAL_ERROR for an unclassified cause.
type EntryError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RemoveFunc is the caller's per-item removal: the domain's existing
// single-item service method, closed over the request's identity, host
// and force flag.
type RemoveFunc func(ctx context.Context, id string) error

// Run removes every requested id through remove and answers one report
// entry per id, in request order. A per-item failure never aborts the
// batch and never changes the endpoint's status code: an
// *appErrors.AppError contributes its own code and message (its
// messages are curated — WithMessage is non-5xx only — so no internal
// detail rides one), and any other cause answers the generic
// INTERNAL_ERROR with the cause logged once and never surfaced.
//
// The report is never nil: an empty id list answers an empty slice, so
// the wire sees [] rather than null.
func Run(ctx context.Context, ids []string, remove RemoveFunc) []Entry {
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		entry := Entry{ID: id, Success: true}
		if err := remove(ctx, id); err != nil {
			entry.Success = false
			entry.Error = Classify(ctx, id, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// Classify maps one item's removal failure onto its report entry: an
// AppError (wrapped included) answers its own code and message, anything
// else the generic internal error — logged once here, because the wire
// entry deliberately drops the cause. Run calls it per item; the
// batch-report domains (images, quadlets) call it directly on the errors
// their one batch call's report carries, so both shapes share one
// classification matrix.
func Classify(ctx context.Context, id string, err error) *EntryError {
	if appErr, ok := errors.AsType[*appErrors.AppError](err); ok {
		return &EntryError{Code: appErr.Code, Message: appErr.Message}
	}
	slog.ErrorContext(ctx, "bulk remove: unexpected per-item error",
		slog.Any("requestID", contextutils.GetRequestID(ctx)),
		slog.Any("id", id),
		slog.Any("error", err))
	return &EntryError{Code: appErrors.ErrInternalError.Code, Message: appErrors.ErrInternalError.Message}
}
