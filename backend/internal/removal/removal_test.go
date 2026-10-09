package removal

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// The module's suite pins the classification matrix once (issue 264):
// every domain's Delete and move source half rides Run, so the matrix
// lives here and nowhere else. The terminals are plain sentinels — the
// module imports no domain vocabulary, so the tests use errors.New
// sentinels and podmanerr-shaped errors (errorhandling.ErrorModel with
// ResponseCode 404/409, plain "no such volume" / "in use" texts).

var (
	errMissing = errors.New("resource not found")
	errRefusal = errors.New("resource refused")
	errBoom    = errors.New("boom")
)

// refusalBuilder is the test's refusal terminal: it threads the cause
// text into a wrap around the refusal sentinel, so the tests can pin
// both the sentinel (errors.Is) and the threaded podman text.
func refusalBuilder(cause string) error {
	return fmt.Errorf("%w: %s", errRefusal, cause)
}

// testTerminals is the matrix's terminal shape: a plain missing value,
// the threading refusal builder, and the shared conflict predicate
// standing in for a domain's refusal predicate.
func testTerminals() Terminals {
	return Terminals{
		Missing:   errMissing,
		Refusal:   refusalBuilder,
		IsRefusal: podmanerr.IsConflict,
	}
}

func TestRun_ClassificationMatrix(t *testing.T) {
	tests := []struct {
		name string
		// remove arms the remove closure; nil leaves it returning nil.
		remove    func() error
		terminals Terminals
		wrapText  string
		wantErr   func(t *testing.T, err error)
	}{
		{
			name:      "clean removal answers nil",
			remove:    func() error { return nil },
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "not-found 404 ErrorModel answers the missing terminal",
			remove: func() error {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such volume"}
			},
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errMissing)
			},
		},
		{
			name: "not-found plain text answers the missing terminal",
			remove: func() error {
				return errors.New("no such volume my-volume")
			},
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errMissing)
			},
		},
		{
			name: "not-found with a nil missing terminal answers nil",
			remove: func() error {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such volume"}
			},
			terminals: Terminals{
				Missing:   nil,
				Refusal:   refusalBuilder,
				IsRefusal: podmanerr.IsConflict,
			},
			wrapText: "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err, "a nil missing terminal is legal: the move source half's vanished-source-is-success")
			},
		},
		{
			name: "refusal 409 ErrorModel answers the refusal terminal with the cause threaded",
			remove: func() error {
				return &errorhandling.ErrorModel{
					ResponseCode: http.StatusConflict,
					Message:      "volume is in use",
					Because:      "used by container web",
				}
			},
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errRefusal)
				assert.Contains(t, err.Error(), "volume is in use used by container web",
					"podmanerr.Message's text must reach the refusal builder")
			},
		},
		{
			name: "refusal plain in-use text answers the refusal terminal with the cause threaded",
			remove: func() error {
				return errors.New("volume is in use by a container")
			},
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errRefusal)
				assert.Contains(t, err.Error(), "volume is in use by a container",
					"the plain refusal text must reach the refusal builder")
			},
		},
		{
			name: "anything else answers the wrap",
			remove: func() error {
				return errBoom
			},
			terminals: testTerminals(),
			wrapText:  "failed to remove volume",
			wantErr: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Equal(t, "failed to remove volume: boom", err.Error())
				assert.ErrorIs(t, err, errBoom, "the wrap must unwrap to the cause")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(tt.remove, tt.terminals, tt.wrapText)
			tt.wantErr(t, err)
		})
	}
}

// TestRun_ClassificationIsTopLevel pins that Run classifies the remove
// closure's own answer, not a nested wrap: a wrapped not-found still
// answers the missing terminal, and a wrapped refusal still answers the
// refusal terminal.
func TestRun_ClassificationIsTopLevel(t *testing.T) {
	err := Run(func() error {
		return fmt.Errorf("engine call: %w", &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such volume"})
	}, testTerminals(), "failed to remove volume")
	require.ErrorIs(t, err, errMissing, "a wrapped not-found must still classify as missing")

	err = Run(func() error {
		return fmt.Errorf("engine call: %w", errors.New("volume is in use by a container"))
	}, testTerminals(), "failed to remove volume")
	require.ErrorIs(t, err, errRefusal, "a wrapped refusal must still classify as refusal")
}
