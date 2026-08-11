package sshexec

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeRunner is a scriptable sessionRunner used to test Service.Exec without
// a live SSH server. It records the cmd it was asked to run and returns the
// preconfigured result/error.
type fakeRunner struct {
	mu        sync.Mutex
	runResult ExecResult
	runErr    error
	closeErr  error

	lastCmd string

	// runCallback, if set, is invoked during Run (used by the
	// context-cancellation test to simulate a slow command).
	runCallback func(ctx context.Context)
}

func (r *fakeRunner) Run(ctx context.Context, cmd string) (ExecResult, error) {
	r.mu.Lock()
	r.lastCmd = cmd
	cb := r.runCallback
	res := r.runResult
	err := r.runErr
	r.mu.Unlock()

	if cb != nil {
		cb(ctx)
	}
	// Honour context cancellation even when no callback is set, so the
	// happy-path fakes still abort on a cancelled ctx.
	if ctx.Err() != nil {
		return ExecResult{}, ctx.Err()
	}
	return res, err
}

func (r *fakeRunner) Close() error { return r.closeErr }

// fakeDialer is a scriptable dialer. Dial returns the configured runner (or
// error); Invalidate records that it was called for a given machineID.
type fakeDialer struct {
	mu sync.Mutex

	dialRunner sessionRunner
	dialErr    error

	invalidated []string

	// dialCount lets tests assert how many real dials happened (cache
	// hit/miss behaviour at the Service level).
	dialCount int
}

func (d *fakeDialer) Dial(_ context.Context, _ string) (sessionRunner, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dialCount++
	if d.dialErr != nil {
		return nil, d.dialErr
	}
	return d.dialRunner, nil
}

func (d *fakeDialer) Invalidate(machineID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.invalidated = append(d.invalidated, machineID)
}

// Close satisfies the dialer interface. The fake never starts a cleanup
// goroutine, so this is a no-op that must be safe to call repeatedly —
// internal/app.App.Close may call it more than once.
func (d *fakeDialer) Close() error { return nil }

// --- Compile-time interface checks ---

func TestService_ImplementsSSHExecInterface(t *testing.T) {
	var _ SSHExecInterface = (*Service)(nil)
}

func TestFakeDialer_ImplementsDialer(t *testing.T) {
	var _ dialer = (*fakeDialer)(nil)
}

func TestFakeRunner_ImplementsSessionRunner(t *testing.T) {
	var _ sessionRunner = (*fakeRunner)(nil)
}

// --- Service.Exec ---

func TestService_Exec_SuccessReturnsRunnerOutput(t *testing.T) {
	runner := &fakeRunner{
		runResult: ExecResult{
			Stdout:   "active\n",
			Stderr:   "",
			ExitCode: 0,
		},
	}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	res, err := svc.Exec(
		context.Background(),
		"machine-1", "user-1", "systemctl",
		ExecOptions{Args: []string{"is-active", "web.service"}},
	)

	assert.NoError(t, err)
	assert.Equal(t, "active\n", res.Stdout)
	assert.Equal(t, "", res.Stderr)
	assert.Equal(t, 0, res.ExitCode)

	// The command + args must be combined and shell-quoted so the
	// remote shell receives exactly what the caller intended.
	assert.Equal(t, "systemctl 'is-active' 'web.service'", runner.lastCmd)
}

func TestService_Exec_NonZeroExitIsNotAnError(t *testing.T) {
	// A non-zero exit code is a legitimate result — the caller interprets
	// it. The SSH layer must surface the exit code without an error so
	// callers can branch on res.ExitCode directly.
	runner := &fakeRunner{
		runResult: ExecResult{
			Stdout:   "",
			Stderr:   "unit not loaded\n",
			ExitCode: 5,
		},
	}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	res, err := svc.Exec(
		context.Background(),
		"machine-1", "user-1", "systemctl",
		ExecOptions{Args: []string{"start", "missing.service"}},
	)

	assert.NoError(t, err, "non-zero exit must NOT be returned as an error")
	assert.Equal(t, 5, res.ExitCode)
	assert.Equal(t, "unit not loaded\n", res.Stderr)
}

func TestService_Exec_DialFailureReturnsErrorAndDoesNotInvalidate(t *testing.T) {
	// A dial failure means we never got a runner; there is nothing cached
	// to invalidate. The Service should propagate the error unwrapped.
	dialErr := errors.New("no route to host")
	d := &fakeDialer{dialErr: dialErr}
	svc := newServiceWithDialer(d)

	res, err := svc.Exec(
		context.Background(),
		"machine-1", "user-1", "echo",
		ExecOptions{},
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to dial machine")
	assert.Equal(t, ExecResult{}, res)
	assert.Empty(t, d.invalidated, "no Invalidate call when dial itself failed")
}

func TestService_Exec_RealRunnerFailureInvalidatesCache(t *testing.T) {
	// A real runner failure (something other than a non-zero exit code)
	// means the cached *ssh.Client is likely stale — the Service MUST
	// call Invalidate so the next call re-dials.
	runner := &fakeRunner{runErr: errors.New("session: connection reset")}
	d := &fakeDialer{dialRunner: runner}
	svc := newServiceWithDialer(d)

	res, err := svc.Exec(
		context.Background(),
		"machine-1", "user-1", "uptime",
		ExecOptions{},
	)

	assert.Error(t, err)
	assert.Equal(t, "session: connection reset", err.Error())
	assert.Equal(t, ExecResult{}, res)
	assert.Equal(t, []string{"machine-1"}, d.invalidated,
		"Invalidate must be called exactly once for the failing machineID")
}

func TestService_Exec_ContextCancellationPropagates(t *testing.T) {
	// When the caller cancels ctx, the runner must observe ctx.Done() and
	// the Service must return ctx.Err() — even if the runner produced a
	// result, cancellation takes precedence (mirrors the production
	// sshSessionRunner's own ctx.Err() check).
	runner := &fakeRunner{
		runResult: ExecResult{Stdout: "partial\n", ExitCode: 0},
	}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the call

	res, err := svc.Exec(ctx, "machine-1", "user-1", "sleep", ExecOptions{Args: []string{"10"}})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, ExecResult{}, res)
}

func TestService_Exec_ShellQuotesArgsSafely(t *testing.T) {
	// An arg containing a single-quote must be escaped so it cannot break
	// out of its single-quoted segment. The standard idiom is to close
	// the quote, escape the literal quote, reopen — `'\''`.
	runner := &fakeRunner{}
	svc := newServiceWithDialer(&fakeDialer{dialRunner: runner})

	_, _ = svc.Exec(
		context.Background(),
		"m", "u", "echo",
		ExecOptions{Args: []string{"it's a trap; rm -rf /"}},
	)

	// The whole arg is a single shell-token; the semicolon must NOT be
	// interpreted as a command separator.
	assert.Equal(t, `echo 'it'\''s a trap; rm -rf /'`, runner.lastCmd)
	assert.True(t, strings.HasPrefix(runner.lastCmd, "echo '"))
}

// slowRunner simulates a command that takes longer than the test's timeout,
// so we can prove ctx cancellation actually interrupts Run.
type slowRunner struct {
	started chan struct{}
}

func (r *slowRunner) Run(ctx context.Context, _ string) (ExecResult, error) {
	close(r.started)
	<-ctx.Done()
	return ExecResult{}, ctx.Err()
}
func (r *slowRunner) Close() error { return nil }

func TestService_Exec_ContextCancellationDuringRun(t *testing.T) {
	runner := &slowRunner{started: make(chan struct{})}
	d := &fakeDialer{dialRunner: runner}
	svc := newServiceWithDialer(d)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := svc.Exec(ctx, "m", "u", "sleep", ExecOptions{Args: []string{"9999"}})

	<-runner.started
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, ExecResult{}, res)
}

// --- Service.Close ---

// TestService_Close_StopsDialerCleanupGoroutine verifies that Close stops
// the production dialer's background cleanup goroutine AND blocks until it
// has exited — mirroring connections.ConnectionManager.Stop. The blocking
// contract is what internal/app.App.Close relies on so the integration
// harness (080-01) doesn't leak goroutines between subtests.
//
// We can't observe the goroutine directly, so we assert Close returns
// without error and is idempotent. The blocking-until-exit guarantee is
// exercised via the deadline-style test in app_test.go.
func TestService_Close_StopsDialerCleanupGoroutine(t *testing.T) {
	// Use a real sshDialer (not a fake) so the cleanup goroutine is
	// actually running. CleanupInterval must be > 0 to start the loop.
	d := newSSHDialer(sshDialerConfig{
		maxSize:         10,
		idleTimeout:     15 * time.Minute,
		cleanupInterval: 50 * time.Millisecond,
	})
	svc := newServiceWithDialer(d)

	// Close must not error and must be idempotent. Idempotency is
	// critical: App.Close may be called multiple times (once via the
	// prod defer, once via the test cleanup), and the second call must
	// not panic on a double-close of stopCh.
	assert.NoError(t, svc.Close())
	assert.NoError(t, svc.Close())
}

// TestService_Close_NoOpWhenDialerHasNoCleanup verifies that Close is safe
// to call on a Service whose dialer was constructed without the cleanup
// goroutine (cleanupInterval == 0). This mirrors the test-dialer paths and
// the production constructor's zero-value default for tests.
func TestService_Close_NoOpWhenDialerHasNoCleanup(t *testing.T) {
	d := newSSHDialer(sshDialerConfig{
		maxSize:         10,
		cleanupInterval: 0,
	})
	svc := newServiceWithDialer(d)

	assert.NotPanics(t, func() {
		_ = svc.Close()
	})
}
