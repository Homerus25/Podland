package quadlets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// The stop-and-remove ritual's own suite (issue 231): a table over the
// terminal matrix — missing × running × force × stop × remove-error —
// driving stopAndRemove directly over the fake engine with
// both callers' terminal shapes. The callers themselves stay pinned by
// their own suites (Delete's service and handler tests, the move
// source-removal tests); this suite pins the ritual's decisions so both
// callers inherit them from one place. The remove arms plain errors: the
// adapter owns folding podman's report shapes into the call's single
// error (issue 263), so the report shape no longer exists at this seam —
// except the adapter's reportError marker, which the two marker-armed
// cases below pin: classification (not-found, running refusal) works
// through the marker exactly as through the bare error.

// deleteRemoveTerminals is Delete's terminal shape: a vanished quadlet is
// the 404 sentinel, a running refusal the bare 400 sentinel (the cause
// dropped today — a dropping closure).
func deleteRemoveTerminals() removeTerminals {
	return removeTerminals{
		missing: appErrors.ErrQuadletNotFound,
		running: func(string) error { return appErrors.ErrQuadletResourceRunning },
	}
}

// moveRemoveTerminals is the move source half's terminal shape: a vanished
// quadlet is success (nil), a running refusal the wrapped sentinel that
// becomes the transfer's partial detail (the cause dropped today).
func moveRemoveTerminals() removeTerminals {
	return removeTerminals{
		missing: nil,
		running: func(string) error {
			return fmt.Errorf("failed to remove quadlet: %w", appErrors.ErrQuadletResourceRunning)
		},
	}
}

func runningQuadlet() *entities.ListQuadlet {
	return &entities.ListQuadlet{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running (5s ago)"}
}

func stoppedQuadlet() *entities.ListQuadlet {
	return &entities.ListQuadlet{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"}
}

func TestQuadletService_StopAndRemove_TerminalMatrix(t *testing.T) {
	tests := []struct {
		name string
		// listed is the list entry the locate finds; nil means the quadlet
		// is missing from the list.
		listed    *entities.ListQuadlet
		force     bool
		stop      bool
		terminals removeTerminals
		// stopFails makes the SSH seam answer a failed systemctl stop.
		stopFails bool
		// removeFn arms the engine's remove; nil leaves it unarmed so an
		// unexpected remove panics the test loudly.
		removeFn func(name string, force bool) error
		wantErr  func(t *testing.T, err error)
		// wantStops is the number of systemctl calls expected.
		wantStops int
		// wantRemoveForce, when non-nil, pins the force the remove call
		// must carry; the removeFn itself asserts the name.
		wantRemoveAssert func(t *testing.T, removed bool, force bool)
	}{
		{
			name:      "missing quadlet answers the delete missing terminal before any remove",
			listed:    nil,
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
			},
		},
		{
			name:      "missing quadlet answers the move missing terminal (nil)",
			listed:    nil,
			force:     false,
			stop:      true,
			terminals: moveRemoveTerminals(),
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err, "a vanished quadlet is the move half's success")
			},
		},
		{
			name:      "running without force and without stop refuses up front",
			listed:    runningQuadlet(),
			force:     false,
			stop:      false,
			terminals: deleteRemoveTerminals(),
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
			},
		},
		{
			name:      "running without force and without stop answers the move running terminal",
			listed:    runningQuadlet(),
			force:     false,
			stop:      false,
			terminals: moveRemoveTerminals(),
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
				assert.Contains(t, err.Error(), "failed to remove quadlet",
					"the move half's running terminal is the wrapped sentinel, byte-identical partial detail")
			},
		},
		{
			name:      "running with stop stops through the seam then removes unforced",
			listed:    runningQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(name string, force bool) error {
				assert.Equal(t, "web.container", name)
				assert.False(t, force, "the remove follows an explicit stop and must not be forced")
				return nil
			},
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
			wantStops: 1,
			wantRemoveAssert: func(t *testing.T, removed bool, force bool) {
				assert.True(t, removed)
				assert.False(t, force)
			},
		},
		{
			name:      "running with force removes over the resource without a stop",
			listed:    runningQuadlet(),
			force:     true,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(name string, force bool) error {
				assert.True(t, force, "force must reach the remove")
				return nil
			},
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
			wantRemoveAssert: func(t *testing.T, removed bool, force bool) {
				assert.True(t, removed)
				assert.True(t, force)
			},
		},
		{
			name:      "stop failure propagates and nothing is removed",
			listed:    runningQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			stopFails: true,
			wantErr: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to stop quadlet before remove")
			},
			wantStops: 1,
		},
		{
			name:      "stopped quadlet removes without a stop even with stop set",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(name string, force bool) error {
				return nil
			},
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
			wantRemoveAssert: func(t *testing.T, removed bool, force bool) {
				assert.True(t, removed)
				assert.False(t, force)
			},
		},
		{
			name:      "remove not-found answers the delete missing terminal",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such quadlet"}
			},
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
			},
		},
		{
			name:      "remove not-found answers the move missing terminal (nil)",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: moveRemoveTerminals(),
			removeFn: func(string, bool) error {
				return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such quadlet"}
			},
			wantErr: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name:      "report-carried not-found answers the delete missing terminal",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return reportError{err: &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such quadlet"}}
			},
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletNotFound,
					"the adapter's reportError marker must classify through to the missing terminal")
			},
		},
		{
			name:      "report-carried running refusal answers the running terminal",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return reportError{err: errors.New("quadlet web.container is running and force is not set, refusing to remove")}
			},
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning,
					"the adapter's reportError marker must classify through to the running terminal")
			},
		},
		{
			name:      "remove running refusal without force answers the running terminal",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return errors.New("quadlet web.container is running and force is not set, refusing to remove")
			},
			wantErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
			},
		},
		{
			name:      "remove running refusal with force wraps the raw cause",
			listed:    stoppedQuadlet(),
			force:     true,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return errors.New("quadlet web.container is running and force is not set, refusing to remove")
			},
			wantErr: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to remove quadlet")
				assert.Contains(t, err.Error(), "refusing to remove",
					"force asked for the removal anyway, so the refusal is a plain wrapped failure")
				assert.NotErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
			},
		},
		{
			name:      "remove generic failure wraps",
			listed:    stoppedQuadlet(),
			force:     false,
			stop:      true,
			terminals: deleteRemoveTerminals(),
			removeFn: func(string, bool) error {
				return errors.New("source read-only")
			},
			wantErr: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to remove quadlet: source read-only")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var removed bool
			var removedForce bool
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					if tt.listed == nil {
						return nil, nil
					}
					return []*entities.ListQuadlet{tt.listed}, nil
				},
			}
			if tt.removeFn != nil {
				inner := tt.removeFn
				engine.removeFn = func(name string, force bool) error {
					removed, removedForce = true, force
					return inner(name, force)
				}
			}

			var sshExec *testutils.MockSSHExec
			if tt.stopFails {
				sshExec = testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
					return sshexec.ExecResult{ExitCode: 1, Stderr: "job failed"}, nil
				})
			} else {
				sshExec = testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, command string, opts sshexec.ExecOptions) (sshexec.ExecResult, error) {
					assert.Equal(t, []string{"--user", "stop", "web.service"}, opts.Args,
						"the stop must cross the lifecycle seam scoped to the SSH login's manager")
					return sshexec.ExecResult{ExitCode: 0}, nil
				})
			}
			service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core"))

			err := service.stopAndRemove(context.Background(), "user", "host", "web.container", tt.force, tt.stop, "quadlets.Delete", tt.terminals)

			if tt.wantErr != nil {
				tt.wantErr(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStops, sshExec.CallCount(), "systemctl call count")
			if tt.wantRemoveAssert != nil {
				tt.wantRemoveAssert(t, removed, removedForce)
			}
		})
	}
}

func TestQuadletService_StopAndRemove_ListFailureWraps(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec(), nil)

	err := service.stopAndRemove(context.Background(), "user", "host", "web.container", false, true, "quadlets.Delete", deleteRemoveTerminals())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list quadlets")
}
