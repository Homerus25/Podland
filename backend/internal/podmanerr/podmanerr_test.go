package podmanerr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// model builds a podman ErrorModel like the bindings decode one from a
// non-2xx service API response.
func model(code int, message, because string) *errorhandling.ErrorModel {
	return &errorhandling.ErrorModel{ResponseCode: code, Message: message, Because: because}
}

// The predicates are independent facts, not a partition: one error can
// satisfy several (a 409 saying "already exists" is both IsConflict and
// IsAlreadyExists). Each table pins the full vocabulary plus the unification
// deltas the extraction settles (marked DELTA).

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed 404", model(http.StatusNotFound, "no such secret", "no such secret"), true},
		{"typed 500 does not match", model(http.StatusInternalServerError, "boom", ""), false},
		{"digs through a wrap", fmt.Errorf("failed to inspect: %w", model(http.StatusNotFound, "gone", "")), true},
		{"tunnel no such volume text", errors.New(`no such volume "gone"`), true},
		{"DELTA containers broadened to the shared no-such phrase", errors.New("no such container"), true},
		{"container not found phrase", errors.New("container not found"), true},
		{"quadlet no such quadlet text", errors.New("no such quadlet: x.container"), true},
		{"image not found phrase", errors.New("image not found"), true},
		{"case-insensitive", errors.New("Quadlet NOT FOUND"), true},
		{"running refusal is not not-found", errors.New("resource is running"), false},
		{"unrelated", assert.AnError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFound(tt.err))
		})
	}
}

func TestIsAlreadyExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"already exists phrase", errors.New("a Quadlet with name x.container already exists, refusing to overwrite"), true},
		{"case-insensitive", errors.New("Quadlet ALREADY EXISTS"), true},
		{"bare exists phrase is not enough", errors.New("file exists"), false},
		{"DELTA secrets cause compare widened to contains", model(http.StatusInternalServerError, "db-password: secret name in use", "secret name in use"), true},
		{"unrelated cause does not match", model(http.StatusInternalServerError, "internal podman error", "boom"), false},
		{"message carrying already exists", model(http.StatusConflict, "volume with name x already exists", ""), true},
		{"unrelated", assert.AnError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsAlreadyExists(tt.err))
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed 409", model(http.StatusConflict, "volume in use", ""), true},
		{"typed 500 does not match by code", model(http.StatusInternalServerError, "boom", ""), false},
		{"DELTA images broadened: used-by phrase", errors.New("image is used by one or more containers"), true},
		{"unable to delete phrase", errors.New("unable to delete image"), true},
		{"in use phrase", errors.New("resource is in use"), true},
		{"secret in-use 500 with message text", model(http.StatusInternalServerError, `secret "db-password" is in use by container 3f2e1d0c`, "secret in use"), true},
		{"case-insensitive", errors.New("Volume IN USE"), true},
		{"already exists is not itself a conflict", errors.New("a Quadlet with name x.container already exists"), false},
		{"unrelated", assert.AnError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsConflict(tt.err))
		})
	}
}

func TestIsClientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed 400", model(http.StatusBadRequest, "volume name invalid", ""), true},
		{"typed 401", model(http.StatusUnauthorized, "unauthorized", ""), true},
		{"typed 499", model(499, "odd client error", ""), true},
		{"typed 404 excluded", model(http.StatusNotFound, "gone", ""), false},
		{"typed 409 excluded", model(http.StatusConflict, "conflict", ""), false},
		{"typed 500 does not match", model(http.StatusInternalServerError, "boom", ""), false},
		{"invalid argument phrase", errors.New("invalid argument: volume name"), true},
		{"names must match phrase", errors.New("names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*"), true},
		{"case-insensitive", errors.New("Invalid Argument"), true},
		{"unrelated", assert.AnError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsClientError(tt.err))
		})
	}
}

func TestMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain error text", errors.New("boom"), "boom"},
		{"model message", model(http.StatusInternalServerError, "human message", ""), "human message"},
		{"message with cause appended", model(http.StatusInternalServerError, "human message", "root cause"), "human message root cause"},
		{"cause only", model(http.StatusInternalServerError, "", "root cause"), "root cause"},
		{"empty model falls back to error text", model(http.StatusInternalServerError, "", ""), ""},
		{"digs through a wrap", fmt.Errorf("failed to delete: %w", model(http.StatusConflict, "in use", "")), "in use"},
		{"whitespace is trimmed", model(http.StatusInternalServerError, "  padded  ", "  cause  "), "padded cause"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Message(tt.err))
		})
	}
}
