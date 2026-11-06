package bulkremove

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_ClassificationMatrix pins the per-item classification once for
// every bulk-remove endpoint: a clean removal is a success entry, an
// AppError contributes its own code and message (wrapped or not), and
// anything else answers the generic INTERNAL_ERROR without leaking the
// cause. The domains keep only their composition pins.
func TestRun_ClassificationMatrix(t *testing.T) {
	refusal := appErrors.NewAppError("RESOURCE_IN_USE", "resource is in use", http.StatusConflict)
	notFound := appErrors.NewAppError("RESOURCE_NOT_FOUND", "resource not found", http.StatusNotFound)
	const internalCause = "dial tcp 10.0.0.1:22: connect: connection refused"

	tests := []struct {
		name        string
		removeErr   error
		wantSuccess bool
		wantCode    string
		wantMessage string
	}{
		{
			name:        "a clean removal is a success entry",
			removeErr:   nil,
			wantSuccess: true,
		},
		{
			name:        "a refusal sentinel contributes its code and message",
			removeErr:   refusal,
			wantCode:    "RESOURCE_IN_USE",
			wantMessage: "resource is in use",
		},
		{
			name:        "a threaded refusal message rides the entry",
			removeErr:   refusal.WithMessage("podman refused: resource is being used"),
			wantCode:    "RESOURCE_IN_USE",
			wantMessage: "podman refused: resource is being used",
		},
		{
			name:        "a not-found sentinel contributes its code and message",
			removeErr:   notFound,
			wantCode:    "RESOURCE_NOT_FOUND",
			wantMessage: "resource not found",
		},
		{
			name:        "a wrapped sentinel is unwrapped",
			removeErr:   fmt.Errorf("failed to remove resource: %w", refusal),
			wantCode:    "RESOURCE_IN_USE",
			wantMessage: "resource is in use",
		},
		{
			name:        "an unclassified cause answers the generic internal error",
			removeErr:   errors.New(internalCause),
			wantCode:    appErrors.ErrInternalError.Code,
			wantMessage: appErrors.ErrInternalError.Message,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := Run(context.Background(), []string{"item-1"}, func(context.Context, string) error {
				return tt.removeErr
			})

			require.Len(t, entries, 1, "one entry per requested id")
			entry := entries[0]
			assert.Equal(t, "item-1", entry.ID, "the entry echoes the requested id")
			assert.Equal(t, tt.wantSuccess, entry.Success)
			if tt.wantSuccess {
				assert.Nil(t, entry.Error, "a success entry carries no error")
				return
			}
			require.NotNil(t, entry.Error, "a failed entry carries an error")
			assert.Equal(t, tt.wantCode, entry.Error.Code)
			assert.Equal(t, tt.wantMessage, entry.Error.Message)
			assert.NotContains(t, entry.Error.Message, internalCause,
				"an internal cause must never leak onto the wire")
		})
	}
}

// TestRun_OneEntryPerIDInRequestOrder pins the report's shape: one entry
// per requested id, in request order, and a per-item failure neither
// aborts the batch nor reorders it.
func TestRun_OneEntryPerIDInRequestOrder(t *testing.T) {
	refusal := appErrors.NewAppError("RESOURCE_IN_USE", "resource is in use", http.StatusConflict)
	ids := []string{"first", "second", "third"}

	entries := Run(context.Background(), ids, func(_ context.Context, id string) error {
		if id == "second" {
			return refusal
		}
		return nil
	})

	require.Len(t, entries, 3)
	assert.Equal(t, []string{"first", "second", "third"}, []string{entries[0].ID, entries[1].ID, entries[2].ID},
		"entries must follow request order")
	assert.True(t, entries[0].Success)
	assert.False(t, entries[1].Success, "the refused item is the only failure")
	require.NotNil(t, entries[1].Error)
	assert.Equal(t, "RESOURCE_IN_USE", entries[1].Error.Code)
	assert.True(t, entries[2].Success, "a per-item failure must not abort the batch")
}

// TestRun_EmptyIDsAnswersEmptyArray pins that the report is never null:
// an empty id list answers an empty, non-nil slice that marshals to [].
func TestRun_EmptyIDsAnswersEmptyArray(t *testing.T) {
	entries := Run(context.Background(), nil, func(context.Context, string) error {
		t.Fatal("an empty id list must not call the removal")
		return nil
	})

	require.NotNil(t, entries, "the report must be an empty array, not null")
	assert.Empty(t, entries)
	encoded, err := json.Marshal(entries)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(encoded))
}

// TestEntry_JSONShape pins the report entry's wire shape once: id plus
// success, and the code/message pair only on a failure.
func TestEntry_JSONShape(t *testing.T) {
	success, err := json.Marshal(Entry{ID: "item-1", Success: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"item-1","success":true}`, string(success))

	failure, err := json.Marshal(Entry{
		ID:      "item-2",
		Success: false,
		Error:   &EntryError{Code: "RESOURCE_IN_USE", Message: "resource is in use"},
	})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"id":"item-2","success":false,"error":{"code":"RESOURCE_IN_USE","message":"resource is in use"}}`,
		string(failure))
}

// TestIDsRequest_Decode pins the ids-only request contract once — the
// body a resource with no force path (secrets) takes. It is the same
// `ids` contract as Request, so the 400s are the same web.Decode
// vocabulary; a body that carries `force` anyway decodes with the field
// ignored, because the endpoint's contract has no force parameter at
// all.
func TestIDsRequest_Decode(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
		wantIDs  []string
	}{
		{
			name:    "ids decode",
			body:    `{"ids":["a","b"]}`,
			wantIDs: []string{"a", "b"},
		},
		{
			name:    "a force field is not part of the contract and is ignored",
			body:    `{"ids":["a"],"force":true}`,
			wantIDs: []string{"a"},
		},
		{
			name:     "a missing ids field is a validation failure",
			body:     `{}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "an empty ids array is a validation failure",
			body:     `{"ids":[]}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "an empty id entry is a validation failure",
			body:     `{"ids":["a",""]}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "a malformed body is an invalid request",
			body:     `{"ids":`,
			wantCode: appErrors.ErrInvalidRequest.Code,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request IDsRequest
			appErr := web.Decode(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)), &request)

			if tt.wantCode != "" {
				require.NotNil(t, appErr, "the body must be refused")
				assert.Equal(t, tt.wantCode, appErr.Code)
				return
			}
			require.Nil(t, appErr, "the body must decode")
			assert.Equal(t, tt.wantIDs, request.IDs)
		})
	}
}

// TestRequest_Decode pins the shared request contract once, through the
// same web.Decode ritual every body-bearing handler crosses: ids is
// required and non-empty with no empty entry, force defaults false, and
// the 400s belong to web.Decode's vocabulary.
func TestRequest_Decode(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCode  string
		wantIDs   []string
		wantForce bool
	}{
		{
			name:      "ids and force decode",
			body:      `{"ids":["a","b"],"force":true}`,
			wantIDs:   []string{"a", "b"},
			wantForce: true,
		},
		{
			name:    "force defaults false",
			body:    `{"ids":["a"]}`,
			wantIDs: []string{"a"},
		},
		{
			name:     "a missing ids field is a validation failure",
			body:     `{"force":true}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "an empty ids array is a validation failure",
			body:     `{"ids":[]}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "an empty id entry is a validation failure",
			body:     `{"ids":["a",""]}`,
			wantCode: appErrors.ErrValidationFailed.Code,
		},
		{
			name:     "a malformed body is an invalid request",
			body:     `{"ids":`,
			wantCode: appErrors.ErrInvalidRequest.Code,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request Request
			appErr := web.Decode(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)), &request)

			if tt.wantCode != "" {
				require.NotNil(t, appErr, "the body must be refused")
				assert.Equal(t, tt.wantCode, appErr.Code)
				return
			}
			require.Nil(t, appErr, "the body must decode")
			assert.Equal(t, tt.wantIDs, request.IDs)
			assert.Equal(t, tt.wantForce, request.Force)
		})
	}
}
