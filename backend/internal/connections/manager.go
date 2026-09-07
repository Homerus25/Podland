// Package connections is the home of the podman connection seam: machine
// lookup, key decryption, dialing with per-machine dedup, and the
// per-machine LRU cache with real SSH teardown.
package connections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Homerus25/podland/backend/internal/encryption"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/bindings"
)

// Manager is the seam every machine-scoped podman call crosses.
type Manager interface {
	// WithConnection hands fn a live podman connection context and
	// releases it when fn returns. Dial failures are logged here
	// (op, machineID, userID, cause) and returned as
	// appErrors.ErrConnectionFailed — callers never map it themselves.
	// A wait cut short by the caller's own context (cancel/deadline)
	// crosses as the context error instead: it is not a machine-side
	// connection failure.
	WithConnection(ctx context.Context, userID, machineID, op string, fn func(connCtx context.Context) error) error
	ProbeMachine(machineID, userID string) error
	TestConnection(ctx context.Context, hostname string, port int, sshUsername, sshPrivateKey, podmanSocketPath string) error
	// RemoveMachine invalidates a machine's cached connection; the SSH
	// session closes once in-flight calls finish.
	RemoveMachine(machineID string)
	Stop()
}

// Manager interface satisfaction is pinned at compile time. During the
// expand phase the concrete manager also satisfies the old
// machines.ConnectionManagerInterface, which the not-yet-migrated machines
// domain still requires until the contract ticket.
var _ Manager = (*ConnectionManager)(nil)

// DefaultConnectionTestTimeout is the registration-probe deadline when no
// explicit timeout is configured (matches the
// MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS=10 default documented in
// description_backend.md).
const DefaultConnectionTestTimeout = 10 * time.Second

const (
	// defaultIdleTimeout is how long an idle connection survives before
	// the cleanup loop closes it.
	defaultIdleTimeout = 15 * time.Minute
	// defaultCleanupInterval is how often the cleanup loop runs.
	defaultCleanupInterval = 30 * time.Second
)

// errMachineRemoved marks an acquisition that lost a race against
// RemoveMachine: the dial (or the freshly dialed connection) must not be
// cached or served. WithConnection maps it to the 502 contract like any
// other acquisition failure.
var errMachineRemoved = errors.New("machine removed during connection acquisition")

// dialFunc is the seam that produces a live podman connection context for
// a persisted machine. Production wires dialForMachine (repo lookup → key
// decrypt → bindings.NewConnectionWithHandle); tests inject a fake (the
// sshexec dialer test pattern) so no live SSH is needed.
type dialFunc func(ctx context.Context, machineID string) (context.Context, bindings.CloseFunc, error)

// ConnectionManager is the production Manager: one module owning
// lookup → decrypt → dial → per-machine LRU cache, with real SSH teardown
// on eviction, idle cleanup, and RemoveMachine.
type ConnectionManager struct {
	machineRepo *models.MachineRepository
	encryption  *encryption.EncryptionService

	maxSize int
	// testTimeout bounds the synchronous connection probes — ProbeMachine
	// for a persisted machine and TestConnection at registration —
	// corresponding to MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS. It is a
	// connection-layer concern: ssh.Dial inside the bindings does not
	// honour a context, so both probes race a goroutine against this
	// deadline.
	testTimeout time.Duration

	idleTimeout     time.Duration
	cleanupInterval time.Duration

	// dial is swapped for a fake in tests (see dialFunc).
	dial dialFunc

	mu      sync.Mutex
	entries map[string]*connectionEntry
	// dying holds graceful-removed entries that still serve in-flight
	// legacy GetConnection/Release pairs (WithConnection callers hold a
	// release func and need no lookup). The handle closes when the last
	// of them releases.
	dying []*connectionEntry

	stopCh chan struct{}
	// stoppedCh is closed by cleanupLoop on its way out so Stop can block
	// until the goroutine has truly exited. nil when no cleanup goroutine
	// was started (cleanupInterval == 0, the test wiring); Stop then
	// doesn't wait.
	stoppedCh chan struct{}
	// stopOnce makes Stop idempotent — internal/app.App.Close() promises
	// "safe to call any number of times"; this keeps Stop safe to call
	// repeatedly without panicking on a double-close of stopCh.
	stopOnce sync.Once
}

// connectionEntry is one cached per-machine connection. An entry is
// pending (ready == false) while its dial is in flight: concurrent cold
// acquisitions wait on done instead of stacking duplicate dials.
type connectionEntry struct {
	machineID string
	connCtx   context.Context
	close     bindings.CloseFunc
	refCount  int
	lastUsed  time.Time
	// dead marks an entry that serves no new users and closes its handle
	// as soon as the last in-flight user releases (RemoveMachine).
	dead bool
	// ready flips when the dial outcome is final: success fills connCtx/
	// close, failure sets dialErr. done is closed alongside so waiters
	// wake and read the outcome.
	ready   bool
	dialErr error
	done    chan struct{}
	// closed pins teardown idempotence at the module level: every close
	// site goes through closeEntry, so the handle fires at most once even
	// if several paths race to tear the entry down.
	closed bool
}

// managerOptions configures a ConnectionManager; NewConnectionManager
// fills the production values.
type managerOptions struct {
	maxSize         int
	testTimeout     time.Duration
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	dial            dialFunc
}

// NewConnectionManager creates the production connection manager. It owns
// everything the dial needs: the machine repository (lookup) and the
// encryption service (key decrypt), plus the cache size limit
// (MAX_CONCURRENT_MACHINES) and the probe deadline
// (MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS, defaulted when <= 0).
func NewConnectionManager(
	machineRepo *models.MachineRepository,
	encryptionService *encryption.EncryptionService,
	maxSize int,
	testTimeout time.Duration,
) *ConnectionManager {
	m := newManager(managerOptions{
		maxSize:         maxSize,
		testTimeout:     testTimeout,
		idleTimeout:     defaultIdleTimeout,
		cleanupInterval: defaultCleanupInterval,
	})
	m.machineRepo = machineRepo
	m.encryption = encryptionService
	m.dial = m.dialForMachine
	return m
}

// newManager builds a manager with explicit options; tests use it to
// inject the fake dial seam and tight timeouts (cleanupInterval == 0
// disables the cleanup goroutine).
func newManager(opts managerOptions) *ConnectionManager {
	m := &ConnectionManager{
		maxSize:         opts.maxSize,
		idleTimeout:     opts.idleTimeout,
		cleanupInterval: opts.cleanupInterval,
		entries:         make(map[string]*connectionEntry),
		stopCh:          make(chan struct{}),
	}
	m.testTimeout = opts.testTimeout
	if m.testTimeout <= 0 {
		m.testTimeout = DefaultConnectionTestTimeout
	}
	m.dial = opts.dial
	if m.dial == nil {
		m.dial = func(context.Context, string) (context.Context, bindings.CloseFunc, error) {
			return nil, nil, errors.New("connections: no dial seam configured")
		}
	}
	if opts.cleanupInterval > 0 {
		m.stoppedCh = make(chan struct{})
		go m.cleanupLoop()
	}
	return m
}

// WithConnection hands fn a live podman connection context and releases it
// when fn returns. Acquisition failures are warn-logged here (op,
// machineID, userID, cause) and returned as appErrors.ErrConnectionFailed
// — callers never map them; fn's own error propagates untouched.
func (m *ConnectionManager) WithConnection(ctx context.Context, userID, machineID, op string, fn func(connCtx context.Context) error) error {
	connCtx, release, err := m.acquire(ctx, machineID, userID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// The caller's own context ended the wait; that is not a
			// machine-side connection failure and must not become a 502.
			return err
		}
		slog.WarnContext(ctx, "Podman connection failed",
			"op", op, "machineID", machineID, "userID", userID, "cause", err)
		return appErrors.ErrConnectionFailed
	}
	defer release()
	return fn(connCtx)
}

// acquire returns a live connection context for machineID plus a release
// func the caller MUST call exactly once when done (WithConnection does it
// via defer; the legacy GetConnection/Release shims hand the pairing to
// the caller instead).
//
// Cache hit: refCount is bumped and the cached context is served. Cache
// miss: a pending entry is registered under the lock and the dial runs
// OUTSIDE it, so cold dials to different machines don't serialize;
// concurrent acquisitions for the same machine wait on the pending
// entry's done channel and share the one dial.
func (m *ConnectionManager) acquire(ctx context.Context, machineID, userID string) (context.Context, func(), error) {
	m.mu.Lock()
	if e, ok := m.entries[machineID]; ok {
		if !e.ready {
			// Dial in flight: wait for its outcome outside the lock.
			m.mu.Unlock()
			select {
			case <-e.done:
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
			m.mu.Lock()
			if e.ready {
				e.refCount++
				e.lastUsed = time.Now()
				connCtx := e.connCtx
				m.mu.Unlock()
				return connCtx, m.releaseFunc(e), nil
			}
			// The shared dial failed (or the machine was removed
			// mid-dial). Fail-fast: the outcome propagates to every
			// waiter, no retry within one request.
			err := e.dialErr
			m.mu.Unlock()
			return nil, nil, err
		}
		e.refCount++
		e.lastUsed = time.Now()
		connCtx := e.connCtx
		m.mu.Unlock()
		return connCtx, m.releaseFunc(e), nil
	}

	// Cache full: evict the least recently used idle connection first.
	// Soft cap, as before: when every entry is in flight nothing is
	// evicted and the dial proceeds.
	if len(m.entries) >= m.maxSize {
		m.evictLRUIdle()
	}

	// Register the pending dial, then dial outside the lock.
	e := &connectionEntry{machineID: machineID, done: make(chan struct{}), lastUsed: time.Now()}
	m.entries[machineID] = e
	m.mu.Unlock()

	connCtx, closeFn, err := m.dial(ctx, machineID)

	m.mu.Lock()
	if err == nil {
		e.close = closeFn
	}
	if err == nil && !e.dead {
		e.connCtx = connCtx
		e.ready = true
		e.refCount = 1
		m.mu.Unlock()
		close(e.done)
		return connCtx, m.releaseFunc(e), nil
	}
	delete(m.entries, machineID)
	if err != nil {
		e.dialErr = err
	} else {
		// The dial won the race against RemoveMachine: the fresh handle
		// closes right away and never serves anyone.
		e.dialErr = errMachineRemoved
		closeEntry(e)
	}
	m.mu.Unlock()
	close(e.done)
	return nil, nil, e.dialErr
}

// releaseFunc returns a once-guarded release for one acquired use. An
// entry whose machine was removed (dead) closes its SSH handle here when
// the last use returns — that is RemoveMachine's graceful teardown.
func (m *ConnectionManager) releaseFunc(e *connectionEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			releaseEntry(e)
		})
	}
}

// releaseEntry drops one in-flight use. Callers must hold m.mu.
func releaseEntry(e *connectionEntry) {
	e.refCount--
	e.lastUsed = time.Now()
	if e.refCount <= 0 && e.dead {
		closeEntry(e)
	}
}

// closeEntry tears an entry's SSH handle down at most once, no matter how
// many paths (release, RemoveMachine, eviction, cleanup) reach it.
// Callers must hold m.mu.
func closeEntry(e *connectionEntry) {
	if e.closed {
		return
	}
	e.closed = true
	e.close()
}

// GetConnection is the legacy expand-phase shim paired with Release: it
// keeps today's semantics (raw errors, caller-side 502 mapping) until each
// domain migrates to WithConnection.
func (m *ConnectionManager) GetConnection(machineID, userID string) (context.Context, error) {
	connCtx, _, err := m.acquire(context.Background(), machineID, userID)
	return connCtx, err
}

// Release is the legacy shim's pairing half: it decrements the reference
// count for machineID, including for entries already moved to the dying
// list by RemoveMachine.
func (m *ConnectionManager) Release(machineID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.entries[machineID]; ok {
		releaseEntry(e)
		return
	}
	for _, e := range m.dying {
		if e.machineID == machineID && e.refCount > 0 {
			releaseEntry(e)
			return
		}
	}
}

// RemoveMachine invalidates a machine's cached connection gracefully: the
// entry leaves the map immediately (fresh acquisitions fail on the machine
// lookup), a dial already in flight completes and closes its fresh handle
// right away, and a ready handle closes when the last in-flight user
// releases.
func (m *ConnectionManager) RemoveMachine(machineID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[machineID]
	if !ok {
		return
	}
	delete(m.entries, machineID)
	e.dead = true
	if !e.ready {
		// Dial in flight: its completion sees dead, closes the fresh
		// handle immediately, and serves nobody.
		return
	}
	if e.refCount <= 0 {
		closeEntry(e)
		return
	}
	// In-flight users hold release funcs (WithConnection) or will call
	// the shim Release (which consults the dying list): the handle closes
	// when the last of them returns.
	m.dying = append(m.dying, e)
	m.pruneDying()
}

// pruneDying drops closed entries from the dying list so it cannot grow
// with every RemoveMachine that had in-flight users. Callers must hold
// m.mu.
func (m *ConnectionManager) pruneDying() {
	kept := m.dying[:0]
	for _, e := range m.dying {
		if e.closed {
			continue
		}
		kept = append(kept, e)
	}
	m.dying = kept
}

// evictLRUIdle closes and removes the least recently used idle entry.
// Active entries (refCount > 0) are never evicted; pending and dead
// entries are skipped. Callers must hold m.mu.
func (m *ConnectionManager) evictLRUIdle() {
	var lru *connectionEntry
	for _, e := range m.entries {
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
	delete(m.entries, lru.machineID)
	closeEntry(lru)
}

// cleanupLoop periodically closes idle connections past the idle timeout.
func (m *ConnectionManager) cleanupLoop() {
	defer close(m.stoppedCh)
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

// cleanup closes connections that haven't been used for the idle timeout.
func (m *ConnectionManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	deadline := time.Now().Add(-m.idleTimeout)
	for machineID, e := range m.entries {
		if e.ready && !e.dead && e.refCount <= 0 && e.lastUsed.Before(deadline) {
			e.dead = true
			delete(m.entries, machineID)
			closeEntry(e)
		}
	}
}

// Stop shuts the idle-cleanup goroutine down and blocks until it has
// terminated. Safe to call any number of times — subsequent calls are
// no-ops that return immediately. App.Close relies on both properties.
func (m *ConnectionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	if m.stoppedCh != nil {
		<-m.stoppedCh
	}
}

// ProbeMachine performs a live status probe of a persisted machine: it
// reuses the cached per-machine connection when one exists (a full SSH
// dial only happens when cold, via acquire) and pings the Podman service
// API through it. The probe is bounded by the same timeout regime as the
// registration TestConnection (testTimeout /
// MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS).
//
// Return contract for the caller (machines.MachineService.GetMachineStatus):
//   - nil: SSH reachable AND the Podman API answered its ping → online.
//   - any error, including context.DeadlineExceeded: offline. There is no
//     third state and the error is the connection layer's own wording —
//     callers must not surface it to API clients.
func (m *ConnectionManager) ProbeMachine(machineID, userID string) error {
	done := make(chan error, 1)
	go func() {
		done <- m.probe(machineID, userID)
	}()

	timer := time.NewTimer(m.testTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return context.DeadlineExceeded
	case err := <-done:
		return err
	}
}

// probe runs the actual dial-or-reuse plus ping. Both legs live inside the
// raced goroutine so a slow cold dial is bounded by testTimeout too, not
// just the ping — matching the registration TestConnection regime.
func (m *ConnectionManager) probe(machineID, userID string) error {
	connCtx, release, err := m.acquire(context.Background(), machineID, userID)
	if err != nil {
		return fmt.Errorf("probe connection: %w", err)
	}
	release()
	return pingPodman(connCtx)
}

// pingPodman issues GET /_ping over an established podman bindings
// connection, mirroring the implicit ping the bindings dial performs. A
// 200 means the Podman service API is answering over the SSH connection.
func pingPodman(ctx context.Context) error {
	conn, err := bindings.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("probe client: %w", err)
	}
	response, err := conn.DoRequest(ctx, nil, http.MethodGet, "/_ping", nil, nil)
	if err != nil {
		return fmt.Errorf("probe ping: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe ping: unexpected status %d", response.StatusCode)
	}
	return nil
}

// TestConnection probes an SSH/podman connection for a not-yet-persisted
// machine, bounding the probe by the configured testTimeout
// (MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS). It is the registration-time
// analogue of acquire: acquire looks up a persisted machine by ID, whereas
// TestConnection dials against raw credentials because the machine row
// does not exist yet (and, per the documented flow, the key has not been
// encrypted yet either). Nothing is cached: the probe only proves the
// target is reachable; the runtime builds its own cached connection on
// first use.
//
// The underlying ssh.Dial (inside bindings.NewConnectionWithHandle) does
// NOT honour the context, so the probe is run in a goroutine raced
// against the deadline. On timeout the goroutine is left to complete on
// its own — for the common fast-failing cases (ECONNREFUSED, bad
// credentials, missing socket) it returns near-instantly, so nothing
// leaks in practice; a pathologically silent host could keep the dial
// alive until the OS TCP timeout, which is an acceptable tradeoff for a
// human-driven registration endpoint.
//
// Return contract for the caller (MachineService.Create):
//   - nil: the target is reachable over SSH AND the podman socket pinged.
//   - context.DeadlineExceeded: the probe exceeded testTimeout. The
//     service maps this to ErrConnectionTestTimeout (400) — detected
//     via errors.Is, so the timeout crosses the package boundary without
//     a shared sentinel.
//   - any other error: a real dial/ping failure (bad host/creds/socket).
//     The service maps this to ErrConnectionTestFailed (400).
func (m *ConnectionManager) TestConnection(
	ctx context.Context,
	hostname string,
	port int,
	sshUsername string,
	sshPrivateKey string,
	podmanSocketPath string,
) error {
	done := make(chan error, 1)
	go func() {
		_, _, dialErr := dialCredentials(ctx, hostname, port, sshUsername, sshPrivateKey, podmanSocketPath)
		done <- dialErr
	}()

	timer := time.NewTimer(m.testTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		// Return the standard deadline error so callers can detect the
		// probe timeout via errors.Is(err, context.DeadlineExceeded)
		// without importing a connections-package sentinel.
		return context.DeadlineExceeded
	case err := <-done:
		return err
	}
}
