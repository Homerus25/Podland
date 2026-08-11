package connections

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/models"
)

// ConnectionManager manages SSH connections to machines
type ConnectionManager struct {
	cache             *ConnectionCache
	machineRepo       *models.MachineRepository
	encryption        *encryption.EncryptionService
	maxConcurrent     int
	queueTimeout      time.Duration
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
) *ConnectionManager {
	return &ConnectionManager{
		cache:             cache,
		machineRepo:       machineRepo,
		encryption:        encryption,
		maxConcurrent:     maxConcurrent,
		queueTimeout:      queueTimeout,
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
