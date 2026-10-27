package connections

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/slogtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/bindings"
)

// Manager is the seam: the concrete manager must satisfy it.
var _ Manager = (*ConnectionManager)(nil)

// The cache-lifecycle contract (dial dedup, no cross-machine serialization,
// LRU-idle eviction, idle cleanup, the RemoveMachine races, Stop) is pinned
// once in internal/rescache's suite; this file keeps the adapter-local
// tests — the WithConnection error mapping and the context detachment — the
// adapter's projections of the lifecycle wiring (RemoveMachine graceful, Stop),
// and the probe tests in manager_probe_test.go.

// --- fake dialer (the sshexec dialer test pattern: inject the dial, no live SSH) ---

// countingDial is a scriptable dial seam. It counts dials and close-handle
// calls per machine and can be told to fail specific machines. Tests that
// need blocking dials or custom contexts build inline closures instead.
type countingDial struct {
	mu        sync.Mutex
	dialCount map[string]int
	closes    map[string]int
	fail      map[string]error
}

func newCountingDial() *countingDial {
	return &countingDial{
		dialCount: map[string]int{},
		closes:    map[string]int{},
		fail:      map[string]error{},
	}
}

func (d *countingDial) dialFunc() dialFunc {
	return func(ctx context.Context, machineID string) (context.Context, bindings.CloseFunc, error) {
		d.mu.Lock()
		d.dialCount[machineID]++
		fail := d.fail[machineID]
		d.mu.Unlock()
		if fail != nil {
			return nil, nil, fail
		}
		return context.Background(), func() {
			d.mu.Lock()
			d.closes[machineID]++
			d.mu.Unlock()
		}, nil
	}
}

func (d *countingDial) dialsOf(machineID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dialCount[machineID]
}

func (d *countingDial) closesOf(machineID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closes[machineID]
}

func (d *countingDial) failMachine(machineID string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail[machineID] = err
}

// newTestManager wires a manager with a fake dial seam and tight timeouts.
// cleanupInterval == 0 disables the idle-cleanup goroutine; tests that
// exercise cleanup pass a tiny interval instead.
func newTestManager(maxSize int, dial dialFunc, idleTimeout, cleanupInterval time.Duration) *ConnectionManager {
	return newManager(managerOptions{
		maxSize:         maxSize,
		testTimeout:     5 * time.Second,
		idleTimeout:     idleTimeout,
		cleanupInterval: cleanupInterval,
		dial:            dial,
	})
}

// --- WithConnection: the seam every machine-scoped podman call crosses ---

func TestWithConnection_RunsFnWithDialValuesAndReleasesOnReturn(t *testing.T) {
	type ctxKey struct{}
	marker := context.WithValue(context.Background(), ctxKey{}, "live-connection")
	closes := atomic.Int32{}
	mgr := newTestManager(10, func(ctx context.Context, machineID string) (context.Context, bindings.CloseFunc, error) {
		return marker, func() { closes.Add(1) }, nil
	}, 20*time.Millisecond, 10*time.Millisecond)

	var got context.Context
	err := mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(connCtx context.Context) error {
		got = connCtx
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, "live-connection", got.Value(ctxKey{}),
		"fn must receive the dial's connection values (the served context is detached from the dialing request, but keeps values)")

	// The release is the manager's job: once fn returns the entry is
	// idle, so the idle-cleanup loop must close the SSH handle without
	// any caller-side Release discipline.
	require.Eventually(t, func() bool { return closes.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"handle must be closed by idle cleanup after fn returns (release-on-return)")
}

func TestWithConnection_PropagatesFnErrorUntouched(t *testing.T) {
	fnErr := errors.New("podman said no")
	dial := newCountingDial()
	mgr := newTestManager(10, dial.dialFunc(), time.Minute, 0)

	err := mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error {
		return fnErr
	})

	require.Error(t, err)
	require.Equal(t, 1, dial.dialsOf("machine-1"), "exactly one dial must back the call")
	assert.Equal(t, fnErr, err, "fn's error must cross untouched — no 502 rewrite, no wrapping")
}

func TestWithConnection_DialFailureLogsCauseAndReturns502(t *testing.T) {
	dialErr := errors.New("ssh: connection refused")
	capture := slogtest.CaptureDefault(t)

	dial := newCountingDial()
	dial.failMachine("machine-1", dialErr)
	mgr := newTestManager(10, dial.dialFunc(), time.Minute, 0)

	fnRan := false
	err := mgr.WithConnection(context.Background(), "user-1", "machine-1", "containers.list", func(context.Context) error {
		fnRan = true
		return nil
	})

	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed,
		"acquisition failure must surface as the 502 CONNECTION_FAILED contract")
	assert.Equal(t, appErrors.ErrConnectionFailed, err, "callers receive the sentinel itself — they never map it")
	assert.False(t, fnRan, "fn must not run without a live connection")

	warns := capture.Attrs(t, slog.LevelWarn)
	require.Len(t, warns, 1, "exactly one warn line with op+machineID+userID+cause")
	assert.Equal(t, "containers.list", warns[0]["op"])
	assert.Equal(t, "machine-1", warns[0]["machineID"])
	assert.Equal(t, "user-1", warns[0]["userID"])
	assert.Contains(t, warns[0]["cause"], dialErr.Error(), "the cause must be logged")
}

func TestWithConnection_CanceledWaitIsNotA502(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	mgr := newTestManager(10, func(context.Context, string) (context.Context, bindings.CloseFunc, error) {
		started <- struct{}{}
		<-release
		return context.Background(), func() {}, nil
	}, time.Minute, 0)

	go func() {
		_ = mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error {
			return nil
		})
	}()
	<-started

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.WithConnection(canceled, "user-1", "machine-1", "test.op", func(context.Context) error {
		return errors.New("must not run")
	})
	close(release)

	assert.ErrorIs(t, err, context.Canceled, "the caller's own cancellation crosses as-is")
	assert.NotErrorIs(t, err, appErrors.ErrConnectionFailed, "a canceled wait is not a machine-side connection failure")
}

// --- the cached context must outlive the dialing request ---

func TestWithConnection_CachedContextOutlivesDialingRequest(t *testing.T) {
	type connKey struct{}
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	var coldCtx, dialedWith context.Context
	mgr := newTestManager(10, func(ctx context.Context, _ string) (context.Context, bindings.CloseFunc, error) {
		dialedWith = ctx
		// Mirrors bindings.NewConnectionWithHandle: the connection
		// context handed back is a value-child of the ctx the dial
		// was handed.
		return context.WithValue(ctx, connKey{}, "podman-client"), func() {}, nil
	}, time.Minute, 0)
	require.NoError(t, mgr.WithConnection(callerCtx, "user-1", "machine-1", "test.op", func(connCtx context.Context) error {
		coldCtx = connCtx
		return nil
	}))

	// The dialing request's handler returned: net/http cancels its
	// context. The dial was handed that ctx, so the dial itself is
	// dead; the connection that got cached must not be.
	cancelCaller()
	require.ErrorIs(t, dialedWith.Err(), context.Canceled,
		"the dial received the caller's context: a canceled caller cancels the dial itself")
	require.NoError(t, coldCtx.Err(),
		"even the cold call's served context is detached: its lifetime is the connection's, not the request's")

	var warmCtx context.Context
	require.NoError(t, mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(connCtx context.Context) error {
		warmCtx = connCtx
		return nil
	}))

	require.NoError(t, warmCtx.Err(),
		"a cache hit after the dialing request died must still hand out a usable context")
	assert.Equal(t, "podman-client", warmCtx.Value(connKey{}),
		"the bindings' client/version values must survive the detach")
}

func TestWithConnection_CanceledCallerAbortsSlowDial(t *testing.T) {
	dialStarted := make(chan struct{})
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	mgr := newTestManager(10, func(ctx context.Context, _ string) (context.Context, bindings.CloseFunc, error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}, time.Minute, 0)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.WithConnection(callerCtx, "user-1", "machine-1", "test.op", func(context.Context) error {
			return errors.New("must not run")
		})
	}()
	<-dialStarted
	cancelCaller()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled, "a canceled caller cancels the slow dial and the wait")
		assert.NotErrorIs(t, err, appErrors.ErrConnectionFailed,
			"the caller's own cancellation is not a machine-side connection failure")
	case <-time.After(2 * time.Second):
		t.Fatal("canceling the caller did not abort the slow dial")
	}
}

// --- adapter projections of the cache lifecycle ---
//
// The lifecycle itself is pinned in internal/rescache's suite; these pin the
// adapter's wiring — that RemoveMachine projects onto the cache's graceful
// Remove (not fail-fast Invalidate) and that Stop projects onto cache.Stop.

func TestRemoveMachine_IsGraceful_InFlightCallFinishesFirst(t *testing.T) {
	dial := newCountingDial()
	mgr := newTestManager(10, dial.dialFunc(), time.Minute, 0)

	fnStarted := make(chan struct{})
	releaseFn := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error {
			close(fnStarted)
			<-releaseFn
			return nil
		})
	}()
	<-fnStarted

	mgr.RemoveMachine("machine-1")

	assert.Zero(t, dial.closesOf("machine-1"),
		"RemoveMachine must project onto the graceful Remove: the handle stays open while a call is in flight (fail-fast Invalidate would close it)")

	close(releaseFn)
	require.NoError(t, <-done)
	require.Eventually(t, func() bool { return dial.closesOf("machine-1") == 1 },
		2*time.Second, 5*time.Millisecond,
		"the handle closes once the last in-flight call returns")
}

func TestStop_JoinsCleanupGoroutineAndIsIdempotent(t *testing.T) {
	dial := newCountingDial()
	mgr := newTestManager(10, dial.dialFunc(), 20*time.Millisecond, 10*time.Millisecond)

	done := make(chan struct{})
	go func() { mgr.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return; cleanup goroutine likely still running")
	}

	assert.NotPanics(t, func() { mgr.Stop() }, "Stop must be safe to call any number of times (App.Close contract)")
}
