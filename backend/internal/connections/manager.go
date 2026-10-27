// Package connections is the home of the podman connection seam: machine
// lookup, key decryption, dialing with per-machine dedup, and the
// per-machine LRU cache with real SSH teardown.
//
// The per-machine cache lifecycle (acquire with dial dedup, refcounted
// release, LRU-idle eviction, idle cleanup, graceful removal) lives in
// internal/rescache (architecture review 2026-10-19, candidate C1); this
// package is the adapter — it hands over its dial and close and keeps its own
// error vocabulary, probe regime, and payload (the connection-context
// detachment, §4, stays in the adapter's dial func).
package connections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/encryption"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/rescache"
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

// Manager interface satisfaction is pinned at compile time.
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

// dialFunc is the seam that produces a live podman connection context for
// a persisted machine. Production wires dialForMachine (repo lookup → key
// decrypt → bindings.NewConnectionWithHandle); tests inject a fake (the
// sshexec dialer test pattern) so no live SSH is needed.
type dialFunc func(ctx context.Context, machineID string) (context.Context, bindings.CloseFunc, error)

// connResource is the cached payload: the detached connection context plus
// its teardown handle.
type connResource struct {
	connCtx context.Context
	closeFn bindings.CloseFunc
}

// ConnectionManager is the production Manager: one module owning
// lookup → decrypt → dial → per-machine cache, with real SSH teardown on
// eviction, idle cleanup, and RemoveMachine. The cache lifecycle itself is
// delegated to internal/rescache; this adapter owns the dial seam, the
// connection-context detachment, the error mapping, and the probe regime.
type ConnectionManager struct {
	machineRepo *models.MachineRepository
	encryption  *encryption.EncryptionService

	// testTimeout bounds the synchronous connection probes — ProbeMachine
	// for a persisted machine and TestConnection at registration —
	// corresponding to MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS. It is a
	// connection-layer concern: ssh.Dial inside the bindings does not
	// honour a context, so both probes race a goroutine against this
	// deadline.
	testTimeout time.Duration

	// cache owns the per-machine connection lifecycle (acquire with dial
	// dedup, refcounted release, LRU-idle eviction, idle cleanup, graceful
	// removal) — internal/rescache (architecture review 2026-10-19, C1).
	cache *rescache.Cache[connResource]

	// dial is swapped for a fake in tests (see dialFunc).
	dial dialFunc
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
	m := &ConnectionManager{}
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
	m.cache = rescache.New(rescache.Options[connResource]{
		Dial:            m.dialResource,
		Close:           closeResource,
		MaxSize:         opts.maxSize,
		IdleTimeout:     opts.idleTimeout,
		CleanupInterval: opts.cleanupInterval,
	})
	return m
}

// dialResource is the rescache Dial seam: it runs the adapter's dial (machine
// lookup → key decrypt → podman-over-SSH dial) and detaches the connection
// context from the dialing request. A request-scoped ctx dies with its HTTP
// handler, which would kill the connection for every later cache hit — the
// stored context must live as long as the connection itself, governed by
// release/refcount, eviction, idle cleanup, and RemoveMachine.
// context.WithoutCancel drops cancellation and deadline but keeps values, so
// the bindings' client/version keys the podman calls rely on survive (§4:
// "connection lifetime is the connection's, not the request's").
// The dial seam (m.dial) is read at call time, not captured at construction,
// so tests can swap it after the manager is built — newProbeManager's
// seed-then-swap relies on this.
func (m *ConnectionManager) dialResource(ctx context.Context, machineID string) (connResource, error) {
	connCtx, closeFn, err := m.dial(ctx, machineID)
	if err != nil {
		return connResource{}, err
	}
	return connResource{connCtx: context.WithoutCancel(connCtx), closeFn: closeFn}, nil
}

// closeResource tears a cached connection down (the SSH handle).
func closeResource(r connResource) {
	r.closeFn()
}

// WithConnection hands fn a live podman connection context and releases it
// when fn returns. Acquisition failures are warn-logged here (op,
// machineID, userID, cause) and returned as appErrors.ErrConnectionFailed
// — callers never map them; fn's own error propagates untouched.
func (m *ConnectionManager) WithConnection(ctx context.Context, userID, machineID, op string, fn func(connCtx context.Context) error) error {
	res, release, err := m.cache.Acquire(ctx, machineID)
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
	return fn(res.connCtx)
}

// RemoveMachine invalidates a machine's cached connection gracefully: the
// entry leaves the cache immediately (fresh acquisitions fail on the machine
// lookup), a dial already in flight completes and closes its fresh handle
// right away, and a ready handle closes when the last in-flight user
// releases.
func (m *ConnectionManager) RemoveMachine(machineID string) {
	m.cache.Remove(machineID)
}

// Stop shuts the idle-cleanup goroutine down and blocks until it has
// terminated. Safe to call any number of times — subsequent calls are
// no-ops that return immediately. App.Close relies on both properties.
func (m *ConnectionManager) Stop() {
	m.cache.Stop()
}

// ProbeMachine performs a live status probe of a persisted machine: it
// reuses the cached per-machine connection when one exists (a full SSH
// dial only happens when cold, via Acquire) and pings the Podman service
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
		done <- m.probe(machineID)
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
func (m *ConnectionManager) probe(machineID string) error {
	res, release, err := m.cache.Acquire(context.Background(), machineID)
	if err != nil {
		return fmt.Errorf("probe connection: %w", err)
	}
	release()
	return pingPodman(res.connCtx)
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

// TestTimeoutError is the typed probe-timeout signal TestConnection returns
// when the registration probe exceeds the configured testTimeout. It
// carries the actual bound so the registration flow can name it in its 400
// message ("Connection test timed out after 10s", issue 399) — a typed
// error rather than the bare context deadline, which carries no bound.
// Detected via errors.As. ProbeMachine (the status endpoint) keeps the
// bare context.DeadlineExceeded: its callers never surface probe detail.
type TestTimeoutError struct {
	// Bound is the configured probe deadline the call exceeded
	// (MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS).
	Bound time.Duration
}

func (e *TestTimeoutError) Error() string {
	return fmt.Sprintf("connection test timed out after %s", e.Bound)
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
//   - *TestTimeoutError: the probe exceeded testTimeout. The service maps
//     this to ErrConnectionTestTimeout (400) with the carried bound in
//     the message — detected via errors.As, so the timeout and its bound
//     cross the package boundary without a shared sentinel.
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
		// Return the typed timeout error carrying the configured bound so
		// callers can name the bound (issue 399) — detected via errors.As,
		// without importing a connections-package sentinel.
		return &TestTimeoutError{Bound: m.testTimeout}
	case err := <-done:
		return err
	}
}
