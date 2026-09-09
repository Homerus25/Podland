package quadlets

import (
	"context"
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
//   - buildLifecycleArgs — systemctl command/arg construction (force/timeout)
//   - interpretLifecycleResult — non-zero exit-code → AppError mapping
//   - buildLifecycleResponse — success-response shape per action

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

func TestBuildLifecycleArgs(t *testing.T) {
	t.Run("start has no force/timeout flags", func(t *testing.T) {
		got := buildLifecycleArgs("start", "web.service", LifecycleOptions{})
		assert.Equal(t, []string{"--user", "start", "web.service"}, got)
	})

	t.Run("stop without options", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{})
		assert.Equal(t, []string{"--user", "stop", "web.service"}, got)
	})

	t.Run("stop with force", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{Force: true})
		assert.Equal(t, []string{"--force", "--user", "stop", "web.service"}, got)
	})

	t.Run("stop with timeout", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{Timeout: 10})
		assert.Equal(t, []string{"--kill-seconds=10", "--user", "stop", "web.service"}, got)
	})

	t.Run("stop with force and timeout", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{Force: true, Timeout: 5})
		assert.Equal(t, []string{"--force", "--kill-seconds=5", "--user", "stop", "web.service"}, got)
	})

	t.Run("restart with force and timeout", func(t *testing.T) {
		got := buildLifecycleArgs("restart", "grp.service", LifecycleOptions{Force: true, Timeout: 30})
		assert.Equal(t, []string{"--force", "--kill-seconds=30", "--user", "restart", "grp.service"}, got)
	})

	t.Run("zero timeout is omitted", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{Timeout: 0})
		assert.Equal(t, []string{"--user", "stop", "web.service"}, got)
	})

	t.Run("negative timeout is omitted", func(t *testing.T) {
		got := buildLifecycleArgs("stop", "web.service", LifecycleOptions{Timeout: -1})
		assert.Equal(t, []string{"--user", "stop", "web.service"}, got)
	})
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
// the service so that, if resolveForLifecycle ever does return, runSystemctl is
// exercised — but under the mock connection that path is not reached.

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
			"the resolved unit name must drive the systemctl invocation")
		return sshexec.ExecResult{ExitCode: 0}, nil
	})
	service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec)

	got, err := service.Start(context.Background(), "user", "machine", "web.container")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"name=web.container"}, listedFilters.Filters,
		"the lookup must filter by name")
	assert.Equal(t, "running", got.State)
	assert.Equal(t, "web.service", got.Unit)
}

func TestQuadletService_Stop_MissingQuadletIs404(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	// The SSH exec stays unarmed: a missing quadlet must not run systemctl.
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec())

	got, err := service.Stop(context.Background(), "user", "machine", "web.container", LifecycleOptions{})

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
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec())

	got, err := service.Restart(context.Background(), "user", "machine", "web.container", LifecycleOptions{})

	require.ErrorIs(t, err, appErrors.ErrQuadletNotRunning)
	assert.Nil(t, got)
}

// TestQuadletService_Lifecycle_NilSSHExec tests the defensive nil-check in
// runSystemctl. When a nil sshExec is configured, the service should return
// ErrInternalError rather than panic.
func TestQuadletService_Lifecycle_NilSSHExec(t *testing.T) {
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil)
	ctx := context.Background()

	result, err := service.runSystemctl(ctx, "test-machine", "test-user", "start", "test.service", LifecycleOptions{})
	assert.Error(t, err, "Should return error when sshExec is nil")
	assert.Equal(t, appErrors.ErrInternalError, err, "Should map to ErrInternalError")
	assert.Equal(t, sshexec.ExecResult{}, result, "Should return empty result")
}

// TestQuadletService_Lifecycle_ServiceConstruction tests that a service can
// be constructed with nil dependencies for defensive programming, matching
// the established pattern in this package.
func TestQuadletService_Lifecycle_ServiceConstruction(t *testing.T) {
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil)
	assert.NotNil(t, service, "Service should construct even with nil sshExec")
	assert.Nil(t, service.sshExec, "sshExec should be nil")
}
