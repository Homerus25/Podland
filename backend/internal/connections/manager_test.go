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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/bindings"
)

// Manager is the seam: the concrete manager must satisfy it.
var _ Manager = (*ConnectionManager)(nil)

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

func TestWithConnection_RunsFnWithDialContextAndReleasesOnReturn(t *testing.T) {
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
	assert.Equal(t, marker, got, "fn must receive the live connection context from the dial")

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
	capture := &captureHandler{}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })

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

	warns := capture.warnAttrs(t)
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

// --- dial dedup replaces queuing ---

func TestWithConnection_ConcurrentColdAcquisitionsShareOneDial(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	dials := atomic.Int32{}
	mgr := newTestManager(10, func(context.Context, string) (context.Context, bindings.CloseFunc, error) {
		dials.Add(1)
		started <- struct{}{}
		<-release
		return context.Background(), func() {}, nil
	}, time.Minute, 0)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	fnsRun := atomic.Int32{}
	for range 2 {
		wg.Go(func() {
			errs <- mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error {
				fnsRun.Add(1)
				return nil
			})
		})
	}

	<-started // the cold dial is in flight, its pending entry already in the map
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	assert.Equal(t, int32(1), dials.Load(), "concurrent cold acquisitions must share one dial")
	assert.Equal(t, int32(2), fnsRun.Load(), "both callbacks must run against the shared connection")
}

func TestWithConnection_ColdDialsToDifferentMachinesDoNotSerialize(t *testing.T) {
	blockA := make(chan struct{})
	startedA := make(chan struct{}, 1)
	startedB := make(chan struct{}, 1)
	mgr := newTestManager(10, func(_ context.Context, machineID string) (context.Context, bindings.CloseFunc, error) {
		switch machineID {
		case "machine-a":
			startedA <- struct{}{}
			<-blockA
		case "machine-b":
			startedB <- struct{}{}
		}
		return context.Background(), func() {}, nil
	}, time.Minute, 0)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = mgr.WithConnection(context.Background(), "user-1", "machine-a", "test.op", func(context.Context) error { return nil })
	}()
	<-startedA
	go func() {
		defer wg.Done()
		_ = mgr.WithConnection(context.Background(), "user-1", "machine-b", "test.op", func(context.Context) error { return nil })
	}()

	select {
	case <-startedB:
		// machine-b's dial started while machine-a's is still blocked:
		// cold dials to different machines don't serialize behind one lock.
	case <-time.After(2 * time.Second):
		close(blockA)
		wg.Wait()
		t.Fatal("cold dial to a second machine serialized behind the first machine's in-flight dial")
	}

	close(blockA)
	wg.Wait()
}

// --- teardown: eviction and idle cleanup close the SSH handle ---

func TestIdleCleanup_ClosesIdleHandle(t *testing.T) {
	dial := newCountingDial()
	mgr := newTestManager(10, dial.dialFunc(), 20*time.Millisecond, 10*time.Millisecond)

	require.NoError(t, mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error { return nil }))
	require.Zero(t, dial.closesOf("machine-1"))

	require.Eventually(t, func() bool { return dial.closesOf("machine-1") == 1 },
		2*time.Second, 5*time.Millisecond,
		"idle cleanup must close the SSH handle of an idle connection")
}

func TestWithConnection_CacheFullEvictsIdleConnectionAndClosesHandle(t *testing.T) {
	dial := newCountingDial()
	mgr := newTestManager(1, dial.dialFunc(), time.Minute, 0)

	require.NoError(t, mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error { return nil }))
	require.Zero(t, dial.closesOf("machine-1"))

	require.NoError(t, mgr.WithConnection(context.Background(), "user-1", "machine-2", "test.op", func(context.Context) error { return nil }))

	assert.Equal(t, 1, dial.closesOf("machine-1"),
		"LRU eviction when the cache is full must close the evicted entry's SSH handle")
}

// --- RemoveMachine: graceful invalidation ---

func TestRemoveMachine_ClosesIdleHandleImmediately(t *testing.T) {
	dial := newCountingDial()
	mgr := newTestManager(10, dial.dialFunc(), time.Minute, 0)
	require.NoError(t, mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error { return nil }))
	require.Zero(t, dial.closesOf("machine-1"))

	mgr.RemoveMachine("machine-1")

	assert.Equal(t, 1, dial.closesOf("machine-1"), "a removed machine's idle handle closes immediately")
}

func TestRemoveMachine_WaitsForInFlightCallbackBeforeClosing(t *testing.T) {
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

	assert.Zero(t, dial.closesOf("machine-1"), "handle must stay open while the callback is in flight")

	// The removed machine's entry left the cache at once: fresh
	// acquisitions fail on the (now missing) machine lookup.
	dial.failMachine("machine-1", errors.New("machine not found"))
	err := mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error { return nil })
	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)

	close(releaseFn)
	require.NoError(t, <-done)
	require.Eventually(t, func() bool { return dial.closesOf("machine-1") == 1 },
		2*time.Second, 5*time.Millisecond,
		"handle closes once the last in-flight callback returns")
}

func TestRemoveMachine_DuringDialClosesTheFreshHandleImmediately(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	closes := atomic.Int32{}
	mgr := newTestManager(10, func(context.Context, string) (context.Context, bindings.CloseFunc, error) {
		started <- struct{}{}
		<-release
		return context.Background(), func() { closes.Add(1) }, nil
	}, time.Minute, 0)

	fnRan := atomic.Bool{}
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.WithConnection(context.Background(), "user-1", "machine-1", "test.op", func(context.Context) error {
			fnRan.Store(true)
			return nil
		})
	}()
	<-started

	mgr.RemoveMachine("machine-1")
	close(release)

	require.Eventually(t, func() bool { return closes.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"a dial already in flight when the machine is removed completes and closes its handle right away")
	assert.ErrorIs(t, <-errCh, appErrors.ErrConnectionFailed,
		"the caller whose dial lost the race gets the 502 contract")
	assert.False(t, fnRan.Load(), "fn must not run for a removed machine")
}

func TestRemoveMachine_UnknownMachineIsNoOp(t *testing.T) {
	mgr := newTestManager(10, newCountingDial().dialFunc(), time.Minute, 0)
	assert.NotPanics(t, func() { mgr.RemoveMachine("no-such-machine") })
}

// --- Stop: idempotent shutdown of the cleanup goroutine ---

func TestStop_ShutsCleanupGoroutineDownIdempotently(t *testing.T) {
	mgr := newTestManager(10, newCountingDial().dialFunc(), 20*time.Millisecond, 10*time.Millisecond)

	done := make(chan struct{})
	go func() { mgr.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return; cleanup goroutine likely still running")
	}

	assert.NotPanics(t, func() { mgr.Stop() }, "Stop must be safe to call any number of times (App.Close contract)")
}

func TestStop_NoCleanupGoroutineReturnsImmediately(t *testing.T) {
	mgr := newTestManager(10, newCountingDial().dialFunc(), time.Minute, 0)
	assert.NotPanics(t, func() { mgr.Stop() })
}

// --- legacy GetConnection/Release shims (expand phase) ---

// The shims keep TODAY's semantics: raw errors, caller-side mapping (the
// compile-time satisfaction of machines.ConnectionManagerInterface is
// pinned in images/service_test.go).
func TestLegacyShims_GetConnectionReleaseKeepRawSemantics(t *testing.T) {
	dialErr := errors.New("ssh: handshake failed")
	dial := newCountingDial()
	dial.failMachine("machine-1", dialErr)
	mgr := newTestManager(10, dial.dialFunc(), time.Minute, 0)

	_, err := mgr.GetConnection("machine-1", "user-1")
	require.Error(t, err)
	assert.Equal(t, dialErr, err,
		"shims surface the raw dial error; 502 mapping stays at the service sites until their migration slice")
	assert.NotErrorIs(t, err, appErrors.ErrConnectionFailed)

	mgr.Release("machine-1") // no cached entry: must be a safe no-op

	ctx, err := mgr.GetConnection("machine-2", "user-1")
	require.NoError(t, err)
	require.NotNil(t, ctx)
	mgr.Release("machine-2")
}

// --- slog capture for the 502 cause-log ownership ---

type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

func (h *captureHandler) warnAttrs(t *testing.T) []map[string]string {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		attrs := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			if err, ok := a.Value.Any().(error); ok {
				attrs[a.Key] = err.Error()
				return true
			}
			attrs[a.Key] = a.Value.String()
			return true
		})
		out = append(out, attrs)
	}
	return out
}
