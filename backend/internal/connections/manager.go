package connections

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Homerus25/podland/backend/internal/encryption"
	"github.com/Homerus25/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/bindings"
)

// ConnectionManager manages SSH connections to machines
type ConnectionManager struct {
	cache         *ConnectionCache
	machineRepo   *models.MachineRepository
	encryption    *encryption.EncryptionService
	maxConcurrent int
	queueTimeout  time.Duration
	// testTimeout bounds the synchronous connection probe performed at
	// machine registration (MachineService.Create → TestConnection),
	// corresponding to MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS. It is a
	// connection-layer concern: ssh.Dial inside the bindings does not
	// honour a context, so TestConnection wraps the dial in a goroutine
	// raced against this deadline.
	testTimeout       time.Duration
	connectionQueue   chan connectionRequest
	mu                sync.Mutex
	activeConnections map[string]bool

	// stopCh signals processQueue to exit. Closed by Stop.
	stopCh chan struct{}
	// stoppedCh is closed by processQueue when it has fully exited,
	// letting Stop block until the goroutine is gone.
	stoppedCh chan struct{}

	// stopOnce makes Stop idempotent — internal/app.App.Close() promises
	// "safe to call any number of times"; this keeps Stop safe to call
	// repeatedly without panicking on a double-close of stopCh.
	stopOnce sync.Once
}

// DefaultConnectionTestTimeout is the registration-probe deadline when no
// explicit timeout is configured (matches the
// MACHINE_CONNECTION_TEST_TIMEOUT_SECONDS=10 default documented in
// description_backend.md).
const DefaultConnectionTestTimeout = 10 * time.Second

type connectionRequest struct {
	machineID string
	userID    string
	response  chan<- connectionResponse
}

type connectionResponse struct {
	connCtx context.Context
	err     error
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(
	cache *ConnectionCache,
	machineRepo *models.MachineRepository,
	encryption *encryption.EncryptionService,
	maxConcurrent int,
	queueTimeout time.Duration,
	testTimeout time.Duration,
) *ConnectionManager {
	if testTimeout <= 0 {
		testTimeout = DefaultConnectionTestTimeout
	}
	return &ConnectionManager{
		cache:             cache,
		machineRepo:       machineRepo,
		encryption:        encryption,
		maxConcurrent:     maxConcurrent,
		queueTimeout:      queueTimeout,
		testTimeout:       testTimeout,
		connectionQueue:   make(chan connectionRequest, maxConcurrent),
		activeConnections: make(map[string]bool),
		stopCh:            make(chan struct{}),
		stoppedCh:         make(chan struct{}),
	}
}

// GetConnection gets a connection context to a machine
func (m *ConnectionManager) GetConnection(machineID, userID string) (context.Context, error) {
	ctx := context.Background()
	// First try to get from cache
	connCtx, err := m.cache.GetOrCreateContext(ctx, machineID, userID)
	if err == nil {
		return connCtx, nil
	}

	// If cache is full or other issue, queue the request
	return m.QueueRequestContext(ctx, machineID, userID)
}

// QueueRequestContext queues a connection request and returns context
func (m *ConnectionManager) QueueRequestContext(ctx context.Context, machineID, userID string) (context.Context, error) {
	responseChan := make(chan connectionResponse, 1)

	select {
	case m.connectionQueue <- connectionRequest{
		machineID: machineID,
		userID:    userID,
		response:  responseChan,
	}:
		// Request queued successfully
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		// Queue is full
		return nil, fmt.Errorf("connection queue is full")
	}

	// Wait for response with timeout
	select {
	case response := <-responseChan:
		if response.err != nil {
			return nil, response.err
		}
		return response.connCtx, nil
	case <-time.After(m.queueTimeout):
		return nil, fmt.Errorf("connection request timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run starts the connection manager
func (m *ConnectionManager) Run() {
	go m.processQueue()
}

// processQueue processes connection requests from the queue. It exits when
// either the queue channel is closed or Stop is called, closing stoppedCh on
// its way out so Stop can block until the goroutine is gone.
func (m *ConnectionManager) processQueue() {
	defer close(m.stoppedCh)
	for {
		select {
		case req, ok := <-m.connectionQueue:
			if !ok {
				return
			}
			connCtx, err := m.cache.GetOrCreateContext(context.Background(), req.machineID, req.userID)

			// Send response
			response := connectionResponse{
				connCtx: connCtx,
				err:     err,
			}

			// Try to send response, but don't block if receiver is gone
			select {
			case req.response <- response:
				// Response sent successfully
			default:
				// Receiver is not listening anymore
				// Connection cleanup is handled automatically by Go's HTTP client
			}
		case <-m.stopCh:
			return
		}
	}
}

// Stop signals the processQueue goroutine to exit and blocks until it has
// terminated. Safe to call any number of times — subsequent calls are no-ops
// that return immediately.
func (m *ConnectionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	<-m.stoppedCh
}

func (m *ConnectionManager) Release(machineID string) {
	m.cache.Release(machineID)
}

// ProbeMachine performs a live status probe of a persisted machine: it
// reuses the cached per-machine connection when one exists (a full SSH
// dial only happens when cold, via GetConnection) and pings the Podman
// service API through it. The probe is bounded by the same timeout regime
// as the registration TestConnection (testTimeout /
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
	connCtx, err := m.GetConnection(machineID, userID)
	if err != nil {
		return fmt.Errorf("probe connection: %w", err)
	}
	defer m.Release(machineID)
	return pingPodman(connCtx)
}

// pingPodman issues GET /_ping over an established podman bindings
// connection, mirroring the implicit ping bindings.NewConnectionWithOptions
// performs at dial time (connection_factory.go). A 200 means the Podman
// service API is answering over the SSH connection.
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
// analogue of GetConnection: GetConnection looks up a persisted machine
// by ID, whereas TestConnection dials against raw credentials because the
// machine row does not exist yet (and, per the documented flow, the key
// has not been encrypted yet either).
//
// The underlying ssh.Dial (inside bindings.NewConnectionWithOptions) does
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
	if m.cache == nil || m.cache.factory == nil {
		return fmt.Errorf("connection manager not initialized")
	}

	done := make(chan error, 1)
	go func() {
		done <- m.cache.factory.TestConnection(ctx, hostname, port, sshUsername, sshPrivateKey, podmanSocketPath)
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
