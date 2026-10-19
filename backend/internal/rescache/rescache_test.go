package rescache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRes is the cached resource for the module suite: an id plus a Close
// counter. The cache's Close func increments the counter.
type testRes struct {
	id     string
	closes atomic.Int32
}

// harness wires a Cache[*testRes] with a scriptable dial. Each key maps to a
// *testRes whose Close calls are counted; dials can be blocked, observed, and
// failed per key. Tests that need none of that just use newHarness and Acquire.
type harness struct {
	cache *Cache[*testRes]

	mu        sync.Mutex
	dials     int
	resources map[string]*testRes
	fail      map[string]error
	block     map[string]chan struct{} // dial waits on this before returning
	started   map[string]chan struct{} // closed when the dial for key starts
}

func newHarness(maxSize int, idleTimeout, cleanupInterval time.Duration) *harness {
	h := &harness{
		resources: map[string]*testRes{},
		fail:      map[string]error{},
		block:     map[string]chan struct{}{},
		started:   map[string]chan struct{}{},
	}
	h.cache = New(Options[*testRes]{
		Dial:            h.dial,
		Close:           func(r *testRes) { r.closes.Add(1) },
		MaxSize:         maxSize,
		IdleTimeout:     idleTimeout,
		CleanupInterval: cleanupInterval,
	})
	return h
}

func (h *harness) dial(ctx context.Context, key string) (*testRes, error) {
	h.mu.Lock()
	h.dials++
	res := &testRes{id: key}
	h.resources[key] = res
	fail := h.fail[key]
	block := h.block[key]
	started := h.started[key]
	h.mu.Unlock()

	if started != nil {
		close(started)
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail != nil {
		return nil, fail
	}
	return res, nil
}

func (h *harness) dialsCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dials
}

func (h *harness) closesOf(key string) int32 {
	h.mu.Lock()
	res := h.resources[key]
	h.mu.Unlock()
	if res == nil {
		return 0
	}
	return res.closes.Load()
}

func (h *harness) failKey(key string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fail[key] = err
}

// blockKey arms a dial for key that blocks until the returned channel is
// closed (or the dial's context ends).
func (h *harness) blockKey(key string) chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan struct{})
	h.block[key] = ch
	return ch
}

// startedChan returns a channel closed when the dial for key starts.
func (h *harness) startedChan(key string) chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan struct{})
	h.started[key] = ch
	return ch
}

// --- dial dedup: concurrent cold acquires for one key share one dial ---

func TestAcquire_ConcurrentColdAcquiresShareOneDial(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	results := make(chan *testRes, 2)
	for range 2 {
		wg.Go(func() {
			res, rel, err := h.cache.Acquire(context.Background(), "k")
			if err != nil {
				errs <- err
				return
			}
			results <- res
			rel()
		})
	}

	<-started // the cold dial is in flight, its pending entry already in the map
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	require.Empty(t, errs)
	r1 := <-results
	r2 := <-results
	assert.Equal(t, 1, h.dialsCount(), "concurrent cold acquires must share one dial")
	assert.Same(t, r1, r2, "both callers must receive the shared resource")
}

func TestAcquire_ColdDialsToDifferentKeysDoNotSerialize(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	blockA := h.blockKey("a")
	startedA := h.startedChan("a")
	startedB := h.startedChan("b")

	var wg sync.WaitGroup
	wg.Go(func() {
		_, rel, _ := h.cache.Acquire(context.Background(), "a")
		rel()
	})
	<-startedA
	wg.Go(func() {
		_, rel, _ := h.cache.Acquire(context.Background(), "b")
		rel()
	})

	select {
	case <-startedB:
		// key-b's dial started while key-a's is still blocked: cold dials
		// to different keys don't serialize behind one lock.
	case <-time.After(2 * time.Second):
		close(blockA)
		wg.Wait()
		t.Fatal("cold dial to a second key serialized behind the first key's in-flight dial")
	}

	close(blockA)
	wg.Wait()
}

func TestAcquire_DialErrorPropagatesRawAndDoesNotCache(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	dialErr := errors.New("connection refused")
	h.failKey("k", dialErr)

	_, _, err := h.cache.Acquire(context.Background(), "k")
	assert.ErrorIs(t, err, dialErr, "dial errors return raw (adapters wrap/map)")

	// The next Acquire dials again — no cached failure.
	_, _, err = h.cache.Acquire(context.Background(), "k")
	assert.ErrorIs(t, err, dialErr)
	assert.Equal(t, 2, h.dialsCount(), "a failed dial must not be cached; the next Acquire retries")
}

func TestAcquire_WaitersOnAFailedSharedDialShareItsOutcomeFailFast(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")
	dialErr := errors.New("connection refused")
	h.failKey("k", dialErr)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, _, err := h.cache.Acquire(context.Background(), "k")
			errs <- err
		})
	}

	<-started // the shared cold dial is in flight; the second Acquire waits on it
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.ErrorIs(t, <-errs, dialErr, "the dialing caller gets the raw dial error")
	assert.ErrorIs(t, <-errs, dialErr, "the waiter shares the one dial's outcome fail-fast")
	assert.Equal(t, 1, h.dialsCount(), "the failed shared dial must not be retried within the burst")
}

func TestAcquire_CallerContextEndingAWaitReturnsCtxErr(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")

	go func() {
		_, rel, _ := h.cache.Acquire(context.Background(), "k")
		rel()
	}()
	<-started

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := h.cache.Acquire(canceled, "k")
	close(release)

	assert.ErrorIs(t, err, context.Canceled, "the caller's own context ending a wait returns ctx.Err()")
}

// --- LRU-idle eviction with the soft cap ---

func TestAcquire_CacheFullEvictsLRUIdleAndClosesIt(t *testing.T) {
	h := newHarness(1, time.Minute, 0)

	resA, relA, err := h.cache.Acquire(context.Background(), "a")
	require.NoError(t, err)
	relA() // a is now idle
	require.Zero(t, h.closesOf("a"))

	resB, relB, err := h.cache.Acquire(context.Background(), "b")
	require.NoError(t, err)
	defer relB()

	assert.Equal(t, int32(1), h.closesOf("a"),
		"LRU eviction when the cache is full must close the evicted entry's resource")
	assert.NotSame(t, resA, resB)
}

func TestAcquire_SoftCapEvictsNothingWhenEveryEntryIsInFlight(t *testing.T) {
	h := newHarness(1, time.Minute, 0)

	_, relA, err := h.cache.Acquire(context.Background(), "a")
	require.NoError(t, err)
	// a stays in flight (relA not called): the cache is full and every
	// entry is in flight, so nothing is evicted and the dial proceeds.

	_, relB, err := h.cache.Acquire(context.Background(), "b")
	require.NoError(t, err)
	defer relB()

	assert.Zero(t, h.closesOf("a"),
		"an in-flight entry must never be evicted, even past the soft cap")
	assert.Equal(t, 2, h.dialsCount(), "the dial proceeds past the soft cap when nothing is evictable")
	relA()
}

// --- idle cleanup: closes past-idle entries, skips in-use entries ---

func TestCleanup_ClosesIdleEntryPastIdleTimeout(t *testing.T) {
	h := newHarness(10, 20*time.Millisecond, 10*time.Millisecond)

	_, rel, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	rel()
	require.Zero(t, h.closesOf("k"))

	require.Eventually(t, func() bool { return h.closesOf("k") == 1 },
		2*time.Second, 5*time.Millisecond,
		"idle cleanup must close the resource of an idle entry past the timeout")
}

func TestCleanup_SkipsInUseEntry(t *testing.T) {
	h := newHarness(10, 20*time.Millisecond, 10*time.Millisecond)

	_, rel, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	// k stays in flight well past the idle timeout.

	require.Never(t, func() bool { return h.closesOf("k") == 1 },
		100*time.Millisecond, 10*time.Millisecond,
		"idle cleanup must never close an in-use entry, even past the timeout")

	rel()
	require.Eventually(t, func() bool { return h.closesOf("k") == 1 },
		2*time.Second, 5*time.Millisecond,
		"once released, the now-idle entry is closed by cleanup")
}

// --- Remove: graceful invalidation ---

func TestRemove_ClosesIdleResourceImmediately(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	_, rel, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	rel()
	require.Zero(t, h.closesOf("k"))

	h.cache.Remove("k")

	assert.Equal(t, int32(1), h.closesOf("k"), "a removed key's idle resource closes immediately")
}

func TestRemove_WaitsForInFlightUsersBeforeClosing(t *testing.T) {
	h := newHarness(10, time.Minute, 0)

	acquired := make(chan *testRes)
	releaseFn := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		res, rel, err := h.cache.Acquire(context.Background(), "k")
		if err != nil {
			done <- err
			return
		}
		acquired <- res
		<-releaseFn
		rel()
		done <- nil
	}()
	res1 := <-acquired

	h.cache.Remove("k")

	assert.Zero(t, res1.closes.Load(), "resource must stay open while a user is in flight")

	// The removed key left the cache at once: a fresh Acquire dials again.
	_, rel2, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	rel2()

	close(releaseFn)
	require.NoError(t, <-done)
	require.Eventually(t, func() bool { return res1.closes.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"resource closes once the last in-flight user releases")
}

func TestRemove_DuringDialClosesTheFreshResourceAndWaitersGetErrRemoved(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")

	errCh := make(chan error, 1)
	go func() {
		_, _, err := h.cache.Acquire(context.Background(), "k")
		errCh <- err
	}()
	<-started

	h.cache.Remove("k")
	close(release)

	require.Eventually(t, func() bool { return h.closesOf("k") == 1 },
		2*time.Second, 5*time.Millisecond,
		"a dial already in flight when the key is removed completes and closes its fresh resource right away")
	assert.ErrorIs(t, <-errCh, ErrRemoved,
		"the caller whose dial lost the race against removal gets ErrRemoved")
}

func TestRemove_DuringDialWaitersGetErrRemoved(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")

	errs := make(chan error, 2)
	go func() {
		_, _, err := h.cache.Acquire(context.Background(), "k")
		errs <- err
	}()
	<-started

	// A second Acquire parks on the pending dial's done channel.
	go func() {
		_, _, err := h.cache.Acquire(context.Background(), "k")
		errs <- err
	}()
	time.Sleep(50 * time.Millisecond)

	h.cache.Remove("k")
	close(release)

	assert.ErrorIs(t, <-errs, ErrRemoved,
		"the dialing caller whose dial lost the removal race gets ErrRemoved")
	assert.ErrorIs(t, <-errs, ErrRemoved,
		"waiters on the pending dial share the ErrRemoved outcome")
	assert.Equal(t, 1, h.dialsCount(), "the removal race must not trigger a second dial")
}

func TestAcquire_StaleDialCompletionDoesNotEvictFreshEntry(t *testing.T) {
	// A pending dial P loses the removal race; a fresh Acquire registers a
	// new entry Q for the same key; P's completion must not delete Q —
	// Q's resource stays cached and P's closes immediately.
	dialStarted := make(chan struct{})
	releaseP := make(chan struct{})
	var dials atomic.Int32
	var firstDialRes atomic.Bool // true while the cached resource is P's

	c := New(Options[*testRes]{
		Dial: func(ctx context.Context, key string) (*testRes, error) {
			res := &testRes{id: key}
			if dials.Add(1) == 1 {
				firstDialRes.Store(true)
				close(dialStarted)
				<-releaseP
			} else {
				firstDialRes.Store(false)
			}
			return res, nil
		},
		Close:   func(r *testRes) { r.closes.Add(1) },
		MaxSize: 10,
	})

	pErr := make(chan error, 1)
	go func() {
		_, _, err := c.Acquire(context.Background(), "k")
		pErr <- err
	}()
	<-dialStarted

	c.Remove("k") // P is dead; the key is free again

	qErr := make(chan error, 1)
	go func() {
		_, rel, err := c.Acquire(context.Background(), "k")
		if err == nil {
			rel()
		}
		qErr <- err
	}()
	time.Sleep(50 * time.Millisecond) // Q registers and becomes ready

	close(releaseP) // P completes; its dead path must not evict Q

	require.ErrorIs(t, <-pErr, ErrRemoved)
	require.NoError(t, <-qErr)

	// Q must still be cached: the next Acquire hits it, with no third dial.
	_, rel, err := c.Acquire(context.Background(), "k")
	require.NoError(t, err)
	defer rel()
	assert.Equal(t, 2, int(dials.Load()),
		"the stale dial's completion must not evict the fresh entry (no re-dial)")
	assert.False(t, firstDialRes.Load(),
		"the cache hit must serve Q's resource, not a re-dial")
}

func TestRemove_UnknownKeyIsNoOp(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	assert.NotPanics(t, func() { h.cache.Remove("no-such-key") })
}

// --- Invalidate: fail-fast ---

func TestInvalidate_ClosesEvenMidFlight(t *testing.T) {
	h := newHarness(10, time.Minute, 0)

	_, rel, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	// k is in flight (rel not called).

	h.cache.Invalidate("k")

	assert.Equal(t, int32(1), h.closesOf("k"),
		"Invalidate closes the resource now, even mid-flight (fail-fast)")

	rel()
	assert.Equal(t, int32(1), h.closesOf("k"),
		"releasing after Invalidate must not close the resource twice")
}

func TestInvalidate_UnknownKeyIsNoOp(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	assert.NotPanics(t, func() { h.cache.Invalidate("no-such-key") })
}

func TestInvalidate_DuringDialClosesFreshResourceAndWaitersGetErrRemoved(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	release := h.blockKey("k")
	started := h.startedChan("k")

	errCh := make(chan error, 1)
	go func() {
		_, _, err := h.cache.Acquire(context.Background(), "k")
		errCh <- err
	}()
	<-started

	h.cache.Invalidate("k")
	close(release)

	require.Eventually(t, func() bool { return h.closesOf("k") == 1 },
		2*time.Second, 5*time.Millisecond,
		"a dial already in flight when the key is invalidated completes and closes its fresh resource right away")
	assert.ErrorIs(t, <-errCh, ErrRemoved,
		"the caller whose dial lost the race against invalidation gets ErrRemoved")
}

func TestInvalidateAll_DropsEverythingAndCloses(t *testing.T) {
	h := newHarness(10, time.Minute, 0)

	_, relA, err := h.cache.Acquire(context.Background(), "a")
	require.NoError(t, err)
	defer relA()
	_, relB, err := h.cache.Acquire(context.Background(), "b")
	require.NoError(t, err)
	defer relB()

	h.cache.InvalidateAll()

	assert.Equal(t, int32(1), h.closesOf("a"), "InvalidateAll closes every cached resource")
	assert.Equal(t, int32(1), h.closesOf("b"), "InvalidateAll closes every cached resource")

	// The cache is empty: a fresh Acquire dials again.
	_, relC, err := h.cache.Acquire(context.Background(), "a")
	require.NoError(t, err)
	defer relC()
	assert.Equal(t, 3, h.dialsCount(), "after InvalidateAll the next Acquire dials fresh")
}

// --- Stop: idempotent shutdown of the cleanup goroutine ---

func TestStop_ShutsCleanupGoroutineDownIdempotently(t *testing.T) {
	h := newHarness(10, 20*time.Millisecond, 10*time.Millisecond)

	done := make(chan struct{})
	go func() { h.cache.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return; cleanup goroutine likely still running")
	}

	assert.NotPanics(t, func() { h.cache.Stop() }, "Stop must be safe to call any number of times")
}

func TestStop_NoCleanupGoroutineReturnsImmediately(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	assert.NotPanics(t, func() { h.cache.Stop() })
}

func TestStop_DoesNotCloseCachedResources(t *testing.T) {
	h := newHarness(10, time.Minute, 0)
	_, rel, err := h.cache.Acquire(context.Background(), "k")
	require.NoError(t, err)
	defer rel()

	h.cache.Stop()

	assert.Zero(t, h.closesOf("k"), "Stop joins the goroutine only; it does not close cached resources")
}
