// Package rescache is the one owner of the per-host cache lifecycle
// (architecture review 2026-10-19, candidate C1; docs/architecture-decisions.md §4).
//
// A Cache is generic over the cached resource: an adapter hands over a Dial
// function and a Close function and gets the whole lifecycle — acquire with
// dial dedup (the dial runs outside the lock), refcounted release, LRU-idle
// eviction with the soft cap, idle cleanup that skips in-use entries,
// graceful Remove, fail-fast Invalidate/InvalidateAll, and an idempotent
// Stop that joins the cleanup goroutine. The module logs nothing; adapters
// log with their own vocabulary.
package rescache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRemoved is returned by Acquire when the key was removed (Remove) or
// invalidated (Invalidate) while the acquisition was in flight: the dial lost
// the race against removal, so the freshly dialed resource was closed
// immediately and serves nobody. Waiters on the pending dial share this
// outcome fail-fast.
var ErrRemoved = errors.New("rescache: key removed during acquisition")

// ReleaseFunc drops one hold on a cached resource. It is once-guarded: a
// second call is a no-op.
type ReleaseFunc func()

// Options configures a Cache. Dial and Close must be non-nil.
type Options[T any] struct {
	// Dial produces a fresh resource for key. It runs OUTSIDE the cache
	// lock, so cold dials to different keys never serialize; concurrent
	// cold Acquires for one key share a single Dial. Dial errors return
	// raw (adapters wrap/map).
	Dial func(ctx context.Context, key string) (T, error)
	// Close tears a resource down. Called at most once per resource.
	Close func(resource T)
	// MaxSize is the soft cap on cached entries: when the cache is full,
	// the least recently used idle entry is evicted first; when every
	// entry is in flight, nothing is evicted and the dial proceeds.
	MaxSize int
	// IdleTimeout is how long an idle entry survives before the cleanup
	// loop closes it.
	IdleTimeout time.Duration
	// CleanupInterval is how often the cleanup loop runs. Zero disables
	// the goroutine.
	CleanupInterval time.Duration
}

// Cache is a per-key cache owning the whole entry lifecycle. It is safe for
// concurrent use.
type Cache[T any] struct {
	dial  func(ctx context.Context, key string) (T, error)
	close func(resource T)

	maxSize         int
	idleTimeout     time.Duration
	cleanupInterval time.Duration

	mu      sync.Mutex
	entries map[string]*entry[T]

	stopCh chan struct{}
	// stoppedCh is closed by cleanupLoop on its way out so Stop can block
	// until the goroutine has truly exited. nil when no cleanup goroutine
	// was started (CleanupInterval == 0); Stop then doesn't wait.
	stoppedCh chan struct{}
	// stopOnce makes Stop idempotent — callers may Stop any number of times
	// without panicking on a double-close of stopCh.
	stopOnce sync.Once
}

// entry is one cached resource. An entry is pending (ready == false) while
// its dial is in flight: concurrent cold Acquires wait on done instead of
// stacking duplicate dials.
type entry[T any] struct {
	key      string
	resource T
	refCount int
	lastUsed time.Time
	// dead marks an entry removed by Remove (or invalidated): it serves no
	// new users and closes its resource as soon as the last in-flight user
	// releases — or, for a pending dial, as soon as the dial completes.
	dead bool
	// ready flips when the dial outcome is final: success fills resource,
	// failure sets dialErr. done is closed alongside so waiters wake and
	// read the outcome.
	ready   bool
	dialErr error
	done    chan struct{}
	// closed pins teardown idempotence: every close site goes through
	// closeEntry, so Close fires at most once even if several paths race
	// to tear the entry down.
	closed bool
}

// New creates a Cache. When CleanupInterval > 0 a background goroutine
// closes entries idle past IdleTimeout; Stop joins it.
func New[T any](opts Options[T]) *Cache[T] {
	c := &Cache[T]{
		dial:            opts.Dial,
		close:           opts.Close,
		maxSize:         opts.MaxSize,
		idleTimeout:     opts.IdleTimeout,
		cleanupInterval: opts.CleanupInterval,
		entries:         make(map[string]*entry[T]),
		stopCh:          make(chan struct{}),
	}
	if opts.CleanupInterval > 0 {
		c.stoppedCh = make(chan struct{})
		go c.cleanupLoop()
	}
	return c
}

// Acquire returns the cached resource for key plus a release func the caller
// MUST call exactly once when done.
//
// Cache hit: refCount is bumped and the cached resource is served. Cache
// miss: a pending entry is registered under the lock and the dial runs
// OUTSIDE it, so cold dials to different keys don't serialize; concurrent
// Acquires for the same key wait on the pending entry's done channel and
// share the one dial's outcome fail-fast (no retry). The caller's own
// context ending a wait returns ctx.Err().
//
// A dial error returns raw. A key removed mid-acquisition returns ErrRemoved.
func (c *Cache[T]) Acquire(ctx context.Context, key string) (T, ReleaseFunc, error) {
	c.mu.Lock()
	if e, ok := c.entries[key]; ok {
		if !e.ready {
			// Dial in flight: wait for its outcome outside the lock.
			c.mu.Unlock()
			select {
			case <-e.done:
			case <-ctx.Done():
				var zero T
				return zero, nil, ctx.Err()
			}
			c.mu.Lock()
			if e.ready {
				e.refCount++
				e.lastUsed = time.Now()
				res := e.resource
				c.mu.Unlock()
				return res, c.releaseFunc(e), nil
			}
			// The shared dial failed (or the key was removed mid-dial).
			// Fail-fast: the outcome propagates to every waiter, no retry.
			err := e.dialErr
			c.mu.Unlock()
			var zero T
			return zero, nil, err
		}
		e.refCount++
		e.lastUsed = time.Now()
		res := e.resource
		c.mu.Unlock()
		return res, c.releaseFunc(e), nil
	}

	// Cache full: evict the least recently used idle entry first. Soft
	// cap: when every entry is in flight nothing is evicted and the dial
	// proceeds.
	if len(c.entries) >= c.maxSize {
		c.evictLRUIdle()
	}

	// Register the pending dial, then dial outside the lock.
	e := &entry[T]{key: key, done: make(chan struct{}), lastUsed: time.Now()}
	c.entries[key] = e
	c.mu.Unlock()

	resource, err := c.dial(ctx, key)

	c.mu.Lock()
	if err == nil {
		e.resource = resource
	}
	if err == nil && !e.dead {
		e.ready = true
		e.refCount = 1
		c.mu.Unlock()
		close(e.done)
		return e.resource, c.releaseFunc(e), nil
	}
	// Delete only the entry this dial created. A Remove/Invalidate may
	// have already dropped e and a fresh Acquire re-registered a new
	// pending entry for the same key — the stale dial's completion must
	// not evict that fresh entry.
	if c.entries[key] == e {
		delete(c.entries, key)
	}
	if err != nil {
		e.dialErr = err
	} else {
		// The dial won the race against removal: the fresh resource closes
		// right away and never serves anyone.
		e.dialErr = ErrRemoved
		c.closeEntry(e)
	}
	c.mu.Unlock()
	close(e.done)
	var zero T
	return zero, nil, e.dialErr
}

// releaseFunc returns a once-guarded release for one acquired use. An entry
// whose key was removed (dead) closes its resource here when the last use
// returns — that is Remove's graceful teardown.
func (c *Cache[T]) releaseFunc(e *entry[T]) ReleaseFunc {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.releaseEntry(e)
		})
	}
}

// releaseEntry drops one in-flight use. Callers must hold c.mu.
func (c *Cache[T]) releaseEntry(e *entry[T]) {
	e.refCount--
	e.lastUsed = time.Now()
	if e.refCount <= 0 && e.dead {
		c.closeEntry(e)
	}
}

// closeEntry tears an entry's resource down at most once, no matter how many
// paths (release, Remove, Invalidate, eviction, cleanup) reach it. Callers
// must hold c.mu. It is only ever called on entries whose resource is set
// (ready entries, or a freshly dialed resource that lost the removal race).
func (c *Cache[T]) closeEntry(e *entry[T]) {
	if e.closed {
		return
	}
	e.closed = true
	c.close(e.resource)
}

// Remove gracefully removes key: the entry leaves the cache immediately
// (fresh Acquires dial again), a ready resource closes when the last
// in-flight user releases, and a dial that loses the race against removal
// closes its fresh resource immediately and serves nobody (waiters get
// ErrRemoved). A no-op for unknown keys.
func (c *Cache[T]) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return
	}
	delete(c.entries, key)
	e.dead = true
	if !e.ready {
		// Dial in flight: its completion sees dead, closes the fresh
		// resource immediately, and serves nobody.
		return
	}
	if e.refCount <= 0 {
		c.closeEntry(e)
		return
	}
	// In-flight users hold release funcs: the resource closes when the last
	// of them returns.
}

// Invalidate fail-fast removes key: a ready resource closes now, even
// mid-flight — the affected caller surfaces the close as its own failure. A
// pending dial loses the race the same way Remove does (its fresh resource
// closes on completion, waiters get ErrRemoved). A no-op for unknown keys.
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return
	}
	delete(c.entries, key)
	e.dead = true
	if !e.ready {
		return
	}
	c.closeEntry(e)
}

// InvalidateAll fail-fast removes every entry: each ready resource closes now,
// even mid-flight. Pending dials lose the race the same way Remove does.
func (c *Cache[T]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, e := range c.entries {
		delete(c.entries, key)
		e.dead = true
		if e.ready {
			c.closeEntry(e)
		}
	}
}

// evictLRUIdle closes and removes the least recently used idle entry.
// In-flight entries (refCount > 0) are never evicted; pending and dead
// entries are skipped. Callers must hold c.mu.
func (c *Cache[T]) evictLRUIdle() {
	var lru *entry[T]
	for _, e := range c.entries {
		if !e.ready || e.dead || e.refCount > 0 {
			continue
		}
		if lru == nil || e.lastUsed.Before(lru.lastUsed) {
			lru = e
		}
	}
	if lru == nil {
		return
	}
	lru.dead = true
	delete(c.entries, lru.key)
	c.closeEntry(lru)
}

// cleanupLoop periodically closes entries idle past the idle timeout.
func (c *Cache[T]) cleanupLoop() {
	defer close(c.stoppedCh)
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup closes entries that haven't been used for the idle timeout,
// skipping in-use entries.
func (c *Cache[T]) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	deadline := time.Now().Add(-c.idleTimeout)
	for key, e := range c.entries {
		if e.ready && !e.dead && e.refCount <= 0 && e.lastUsed.Before(deadline) {
			e.dead = true
			delete(c.entries, key)
			c.closeEntry(e)
		}
	}
}

// Stop shuts the idle-cleanup goroutine down and blocks until it has
// terminated. It does NOT close cached resources. Safe to call any number of
// times — subsequent calls are no-ops that return immediately.
func (c *Cache[T]) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
	if c.stoppedCh != nil {
		<-c.stoppedCh
	}
}
