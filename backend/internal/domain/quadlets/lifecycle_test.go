package quadlets

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// --- Pure helpers used by the lifecycle (Start/Stop/Restart) slice ---
//
// As with the other service seams, the podman engine boundary (QuadletList,
// used for the existence check + unit-name resolution) is built inline and
// cannot be exercised under the mock connection. The meaningful lifecycle
// domain logic is therefore extracted into pure functions and tested directly:
//
//   - resolveUnitName — unit-name resolution (prefer UnitName, derive fallback)
//   - checkLifecyclePrecondition — wrong-state guard per action
//   - interpretLifecycleResult — non-zero exit-code → AppError mapping
//   - buildLifecycleResponse — success-response shape per action
//
// The systemctl argv grammar is pinned once at the applyUnitAction seam (the
// action × options table below); the Start/Stop/Restart flow tests cover
// resolution, guards, and response shape without re-asserting argv.

func TestResolveUnitName(t *testing.T) {
	t.Run("prefers UnitName when present", func(t *testing.T) {
		q := &entities.ListQuadlet{Name: "web.container", UnitName: "web.service"}
		assert.Equal(t, "web.service", resolveUnitName(q, "web.container"))
	})

	t.Run("derives <base>.service when UnitName empty", func(t *testing.T) {
		q := &entities.ListQuadlet{Name: "web.container", UnitName: ""}
		assert.Equal(t, "web.service", resolveUnitName(q, "web.container"))
	})

	t.Run("derives for pod resource", func(t *testing.T) {
		q := &entities.ListQuadlet{Name: "grp.pod", UnitName: ""}
		assert.Equal(t, "grp.service", resolveUnitName(q, "grp.pod"))
	})

	t.Run("name without extension appends .service", func(t *testing.T) {
		q := &entities.ListQuadlet{Name: "barename", UnitName: ""}
		assert.Equal(t, "barename.service", resolveUnitName(q, "barename"))
	})
}

func TestDeriveServiceUnit(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"container quadlet", "web.container", "web.service"},
		{"pod quadlet", "grp.pod", "grp.service"},
		{"volume quadlet", "data.volume", "data.service"},
		{"network quadlet", "net.network", "net.service"},
		{"no extension", "barename", "barename.service"},
		{"dotted name", "my.app.container", "my.app.service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveServiceUnit(tt.in))
		})
	}
}

func TestCheckLifecyclePrecondition(t *testing.T) {
	t.Run("start of stopped resource is allowed", func(t *testing.T) {
		assert.NoError(t, actionStart.checkPrecondition("stopped"))
	})

	t.Run("start of unknown state is allowed", func(t *testing.T) {
		assert.NoError(t, actionStart.checkPrecondition("unknown"))
	})

	t.Run("start of running resource is rejected", func(t *testing.T) {
		assert.Equal(t, appErrors.ErrQuadletAlreadyRunning,
			actionStart.checkPrecondition("running"))
	})

	t.Run("stop of running resource is allowed", func(t *testing.T) {
		assert.NoError(t, actionStop.checkPrecondition("running"))
	})

	t.Run("stop of stopped resource is rejected", func(t *testing.T) {
		assert.Equal(t, appErrors.ErrQuadletNotRunning,
			actionStop.checkPrecondition("stopped"))
	})

	t.Run("stop of unknown state is rejected", func(t *testing.T) {
		assert.Equal(t, appErrors.ErrQuadletNotRunning,
			actionStop.checkPrecondition("unknown"))
	})

	t.Run("restart of running resource is allowed", func(t *testing.T) {
		assert.NoError(t, actionRestart.checkPrecondition("running"))
	})

	t.Run("restart of stopped resource is rejected", func(t *testing.T) {
		assert.Equal(t, appErrors.ErrQuadletNotRunning,
			actionRestart.checkPrecondition("stopped"))
	})

	t.Run("start of error state is allowed (attempt recovery)", func(t *testing.T) {
		assert.NoError(t, actionStart.checkPrecondition("error"))
	})
}

// TestQuadletService_ApplyUnitAction_ArgvTable pins the systemctl argv
// grammar exactly once, at the applyUnitAction seam, through the mock SSH
// exec: one row per action × options × SSH-login combination. Nothing below
// this seam (handler tests included) re-asserts argv. The grammar has no
// force flag — systemctl's --force is ignored for stop/restart and the param
// was removed from the contract (issue 205); the handler tests pin that a
// `force=true` query is inert on the HTTP surface. The login dimension pins
// the scoping rule: a non-root SSH login targets the user manager
// (`--user`), a root login targets the system manager (no flag).
func TestQuadletService_ApplyUnitAction_ArgvTable(t *testing.T) {
	tests := []struct {
		name        string
		sshUser     string
		action      lifecycleAction
		unit        string
		opts        LifecycleOptions
		wantCommand string
		wantArgs    []string
	}{
		{
			name:    "start has no options",
			sshUser: "core", action: actionStart, unit: "web.service",
			wantCommand: "systemctl",
			wantArgs:    []string{"--user", "start", "web.service"},
		},
		{
			name:    "stop without options",
			sshUser: "core", action: actionStop, unit: "web.service",
			wantCommand: "systemctl",
			wantArgs:    []string{"--user", "stop", "web.service"},
		},
		{
			name:    "stop with timeout wraps in the coreutils timeout",
			sshUser: "core", action: actionStop, unit: "web.service",
			opts:        LifecycleOptions{Timeout: 10},
			wantCommand: "timeout",
			wantArgs:    []string{"10", "systemctl", "--user", "stop", "web.service"},
		},
		{
			name:    "restart with timeout is one wrapped call",
			sshUser: "core", action: actionRestart, unit: "grp.service",
			opts:        LifecycleOptions{Timeout: 30},
			wantCommand: "timeout",
			wantArgs:    []string{"30", "systemctl", "--user", "restart", "grp.service"},
		},
		{
			name:    "zero timeout is omitted",
			sshUser: "core", action: actionStop, unit: "web.service",
			opts:        LifecycleOptions{Timeout: 0},
			wantCommand: "systemctl",
			wantArgs:    []string{"--user", "stop", "web.service"},
		},
		{
			name:    "negative timeout is omitted",
			sshUser: "core", action: actionStop, unit: "web.service",
			opts:        LifecycleOptions{Timeout: -1},
			wantCommand: "systemctl",
			wantArgs:    []string{"--user", "stop", "web.service"},
		},
		{
			name:    "root start omits --user",
			sshUser: "root", action: actionStart, unit: "web.service",
			wantCommand: "systemctl",
			wantArgs:    []string{"start", "web.service"},
		},
		{
			name:    "root stop omits --user",
			sshUser: "root", action: actionStop, unit: "web.service",
			wantCommand: "systemctl",
			wantArgs:    []string{"stop", "web.service"},
		},
		{
			name:    "root stop with timeout wraps without --user",
			sshUser: "root", action: actionStop, unit: "web.service",
			opts:        LifecycleOptions{Timeout: 10},
			wantCommand: "timeout",
			wantArgs:    []string{"10", "systemctl", "stop", "web.service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCommand string
			var gotArgs []string
			sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, command string, opts sshexec.ExecOptions) (sshexec.ExecResult, error) {
				gotCommand, gotArgs = command, opts.Args
				return sshexec.ExecResult{ExitCode: 0}, nil
			})
			// The engine stays unarmed: applyUnitAction crosses only the SSH seam.
			service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, sshExec, lookupForUser(tt.sshUser))

			err := service.applyUnitAction(context.Background(), "user", "host", tt.action, tt.unit, tt.opts)

			require.NoError(t, err)
			assert.Equal(t, tt.wantCommand, gotCommand)
			assert.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}

// TestQuadletService_ApplyUnitAction_WrapperExitCodes pins the interpretation
// of the coreutils timeout wrapper's exit codes: expiry (124) answers the
// QUADLET_STOP_TIMED_OUT sentinel, the wrapper's own failures (125/126/127)
// keep the generic 500 mapping, and a real systemctl exit code (5: unit not
// loaded) still routes through the stderr heuristics even under the wrapper.
// Without a timeout no wrapper ran, so 124 is NOT the sentinel.
func TestQuadletService_ApplyUnitAction_WrapperExitCodes(t *testing.T) {
	tests := []struct {
		name      string
		opts      LifecycleOptions
		exitCode  int
		stderr    string
		assertErr func(t *testing.T, err error)
	}{
		{
			name:     "expiry 124 maps to the stop-timed-out sentinel",
			opts:     LifecycleOptions{Timeout: 1},
			exitCode: 124,
			assertErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletStopTimedOut)
				var appErr *appErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, http.StatusGatewayTimeout, appErr.Status)
			},
		},
		{
			name:     "restart expiry 124 maps to the same sentinel",
			opts:     LifecycleOptions{Timeout: 3},
			exitCode: 124,
			assertErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletStopTimedOut)
			},
		},
		{
			name:     "wrapper failure 125 keeps the generic mapping",
			opts:     LifecycleOptions{Timeout: 1},
			exitCode: 125,
			stderr:   "timeout: failed to run command",
			assertErr: func(t *testing.T, err error) {
				require.Error(t, err)
				var appErr *appErrors.AppError
				assert.False(t, errors.As(err, &appErr),
					"wrapper failures must stay generic (500), not map to a domain sentinel")
			},
		},
		{
			name:     "wrapper failure 126 keeps the generic mapping",
			opts:     LifecycleOptions{Timeout: 1},
			exitCode: 126,
			stderr:   "timeout: failed to run command 'systemctl': Permission denied",
			assertErr: func(t *testing.T, err error) {
				var appErr *appErrors.AppError
				assert.False(t, errors.As(err, &appErr))
			},
		},
		{
			name:     "wrapper failure 127 keeps the generic mapping",
			opts:     LifecycleOptions{Timeout: 1},
			exitCode: 127,
			stderr:   "timeout: failed to run command 'systemctl': No such file or directory",
			assertErr: func(t *testing.T, err error) {
				var appErr *appErrors.AppError
				assert.False(t, errors.As(err, &appErr),
					"the shell's 'no such' wording must not leak into the not-found heuristic")
			},
		},
		{
			name:     "systemctl exit 5 under the wrapper still maps to not found",
			opts:     LifecycleOptions{Timeout: 10},
			exitCode: 5,
			stderr:   "Failed to stop web.service: Unit web.service not loaded.",
			assertErr: func(t *testing.T, err error) {
				require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
			},
		},
		{
			name:     "exit 124 without the wrapper is not the sentinel",
			opts:     LifecycleOptions{},
			exitCode: 124,
			assertErr: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.NotErrorIs(t, err, appErrors.ErrQuadletStopTimedOut)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
				return sshexec.ExecResult{ExitCode: tt.exitCode, Stderr: tt.stderr}, nil
			})
			service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, sshExec, lookupForUser("core"))

			err := service.applyUnitAction(context.Background(), "user", "host", actionStop, "web.service", tt.opts)

			tt.assertErr(t, err)
		})
	}
}

func TestInterpretLifecycleResult(t *testing.T) {
	t.Run("exit zero is success", func(t *testing.T) {
		assert.NoError(t, actionStart.interpretResult(sshexec.ExecResult{ExitCode: 0}))
	})

	t.Run("exit 5 (unit not loaded) maps to not found", func(t *testing.T) {
		err := actionStart.interpretResult(sshexec.ExecResult{
			ExitCode: 5,
			Stderr:   "Failed to start web.service: Unit web.service not found.",
		})
		assert.Equal(t, appErrors.ErrQuadletNotFound, err)
	})

	t.Run("stderr 'not found' maps to not found regardless of exit code", func(t *testing.T) {
		err := actionStop.interpretResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "Unit web.service is not loaded: no such file",
		})
		assert.Equal(t, appErrors.ErrQuadletNotFound, err)
	})

	t.Run("start with 'already active' stderr maps to already running", func(t *testing.T) {
		err := actionStart.interpretResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "Unit is already active",
		})
		assert.Equal(t, appErrors.ErrQuadletAlreadyRunning, err)
	})

	t.Run("stop with 'inactive' stderr maps to not running", func(t *testing.T) {
		err := actionStop.interpretResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "Unit is inactive",
		})
		assert.Equal(t, appErrors.ErrQuadletNotRunning, err)
	})

	t.Run("restart with 'not running' stderr maps to not running", func(t *testing.T) {
		err := actionRestart.interpretResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "Unit is not running",
		})
		assert.Equal(t, appErrors.ErrQuadletNotRunning, err)
	})

	t.Run("unrecognized failure is a generic error", func(t *testing.T) {
		err := actionStop.interpretResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "something exploded",
		})
		assert.Error(t, err)
		assert.NotEqual(t, appErrors.ErrQuadletNotFound, err)
		assert.NotEqual(t, appErrors.ErrQuadletNotRunning, err)
		assert.NotEqual(t, appErrors.ErrQuadletAlreadyRunning, err)
		assert.Contains(t, err.Error(), "systemctl")
	})
}

func TestBuildLifecycleResponse(t *testing.T) {
	t.Run("start response is running", func(t *testing.T) {
		got := actionStart.buildResponse("web.container", "web.service")
		assert.Equal(t, &QuadletLifecycleResponse{
			ID:    "web.container",
			State: "running",
			Unit:  "web.service",
		}, got)
	})

	t.Run("stop response is stopped", func(t *testing.T) {
		got := actionStop.buildResponse("web.container", "web.service")
		assert.Equal(t, &QuadletLifecycleResponse{
			ID:    "web.container",
			State: "stopped",
			Unit:  "web.service",
		}, got)
	})

	t.Run("restart response is running with restartedAt", func(t *testing.T) {
		got := actionRestart.buildResponse("web.container", "web.service")
		assert.Equal(t, "running", got.State)
		assert.Equal(t, "web.container", got.ID)
		assert.Equal(t, "web.service", got.Unit)
		assert.NotEmpty(t, got.RestartedAt, "restart response must carry a restartedAt timestamp")
	})
}

// --- Service seam: Start / Stop / Restart ---
//
// As with the other service seams in this package, the podman engine boundary
// (QuadletList, used for the existence check + unit-name resolution in
// resolveForLifecycle) is built inline and cannot be exercised under the mock
// connection. The service-level tests therefore cover only the invocation
// shape: the call reaches QuadletList but fails because there is no real
// podman backend behind the mock connection context — the error is NOT a
// connection error, proving we got past the connection seam (whose failure
// mapping is pinned once in the connections package tests).
//
// The domain logic this slice introduces (unit-name resolution, the wrong-state
// guard, systemctl command construction, exit-code interpretation) is covered
// directly on the pure functions above. An SSH-exec mock is still wired into
// the service so that, if resolveForLifecycle ever does return, applyUnitAction
// is exercised — but under the mock connection that path is not reached.

// TestQuadletService_Start_ResolvesUnitThenRunsSystemctl pins the lifecycle
// flow end to end: the quadlet lookup runs through the engine provider, the
// resolved unit name drives `systemctl --user start <unit>` over SSH, and
// the response reports the running state.
func TestQuadletService_Start_ResolvesUnitThenRunsSystemctl(t *testing.T) {
	var listedFilters entities.QuadletListOptions
	engine := &fakeQuadletEngine{
		listFn: func(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			listedFilters = opts
			return []*entities.ListQuadlet{{
				Name:     "web.container",
				UnitName: "web.service",
				Path:     "/x/web.container",
				Status:   "stopped",
			}}, nil
		},
	}
	sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, opts sshexec.ExecOptions) (sshexec.ExecResult, error) {
		assert.Equal(t, []string{"--user", "start", "web.service"}, opts.Args,
			"the resolved unit name must drive the systemctl invocation, scoped to the non-root SSH user's manager")
		return sshexec.ExecResult{ExitCode: 0}, nil
	})
	hosts := lookupForUser("core")
	service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, hosts)

	got, err := service.Start(context.Background(), "user", "host", "web.container")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"name=web.container"}, listedFilters.Filters,
		"the lookup must filter by name")
	assert.Equal(t, "running", got.State)
	assert.Equal(t, "web.service", got.Unit)
	assert.Equal(t, "host", hosts.lastID,
		"the systemctl scope must be resolved for the host being acted on")
}

// TestQuadletService_Start_RootSSHUserOmitsUserFlag pins the root half of
// the scoping rule: when the host's registered SSH login is root, the
// lifecycle addresses the system manager and `--user` must NOT be passed
// (root has no user manager owning its units).
func TestQuadletService_Start_RootSSHUserOmitsUserFlag(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{
				Name:     "web.container",
				UnitName: "web.service",
				Path:     "/x/web.container",
				Status:   "stopped",
			}}, nil
		},
	}
	sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, opts sshexec.ExecOptions) (sshexec.ExecResult, error) {
		assert.Equal(t, []string{"start", "web.service"}, opts.Args,
			"a root SSH login must target the system manager, without --user")
		return sshexec.ExecResult{ExitCode: 0}, nil
	})
	service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("root"))

	got, err := service.Start(context.Background(), "user", "host", "web.container")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "running", got.State)
}

func TestQuadletService_Stop_MissingQuadletIs404(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	// The SSH exec stays unarmed: a missing quadlet must not run systemctl.
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec(), nil)

	got, err := service.Stop(context.Background(), "user", "host", "web.container", LifecycleOptions{})

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_Restart_NonRunningRefused(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container", Status: "stopped"}}, nil
		},
	}
	// The SSH exec stays unarmed: a wrong-state quadlet must not run systemctl.
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec(), nil)

	got, err := service.Restart(context.Background(), "user", "host", "web.container", LifecycleOptions{})

	require.ErrorIs(t, err, appErrors.ErrQuadletNotRunning)
	assert.Nil(t, got)
}

// TestQuadletService_Lifecycle_NilSSHExec tests the defensive nil-check in
// execSystemctl (shared by applyUnitAction and reloadUserSystemd). When a nil
// sshExec is configured, the service should return ErrInternalError rather
// than panic. The host lookup is armed so the scope resolution succeeds
// and the call reaches the nil-checked exec seam.
func TestQuadletService_Lifecycle_NilSSHExec(t *testing.T) {
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, lookupForUser("core"))
	ctx := context.Background()

	err := service.applyUnitAction(ctx, "test-user", "test-host", actionStart, "test.service", LifecycleOptions{})
	assert.Error(t, err, "Should return error when sshExec is nil")
	assert.Equal(t, appErrors.ErrInternalError, err, "Should map to ErrInternalError")
}

// TestQuadletService_Lifecycle_NilHostLookup pins the defensive nil-check
// in systemctlUserScope: a nil host lookup is a programming error and
// answers ErrInternalError rather than panicking, before anything crosses
// the SSH seam.
func TestQuadletService_Lifecycle_NilHostLookup(t *testing.T) {
	// The SSH exec stays unarmed: a nil lookup must fail before any exec.
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, testutils.NewMockSSHExec(), nil)
	ctx := context.Background()

	err := service.applyUnitAction(ctx, "test-user", "test-host", actionStart, "test.service", LifecycleOptions{})
	assert.Error(t, err, "Should return error when host lookup is nil")
	assert.Equal(t, appErrors.ErrInternalError, err, "Should map to ErrInternalError")
}

// TestSystemctlUserScope_LookupFailure pins the error mapping of the scope
// resolution itself: a host deleted concurrently with the request is the
// 404 sentinel, a store failure wraps ErrInternalError.
func TestSystemctlUserScope_LookupFailure(t *testing.T) {
	t.Run("host vanished mid-request maps to not found", func(t *testing.T) {
		service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, &fakeHostLookup{})

		_, err := service.systemctlUserScope("gone-host")
		require.ErrorIs(t, err, appErrors.ErrHostNotFound)
	})

	t.Run("store failure wraps internal error", func(t *testing.T) {
		service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil,
			&fakeHostLookup{err: errors.New("db is down")})

		_, err := service.systemctlUserScope("host")
		require.ErrorIs(t, err, appErrors.ErrInternalError)
	})
}

// TestQuadletService_Lifecycle_ServiceConstruction tests that a service can
// be constructed with nil dependencies for defensive programming, matching
// the established pattern in this package.
func TestQuadletService_Lifecycle_ServiceConstruction(t *testing.T) {
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, nil)
	assert.NotNil(t, service, "Service should construct even with nil sshExec")
	assert.Nil(t, service.sshExec, "sshExec should be nil")
}
