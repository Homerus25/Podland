package connections

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.podman.io/podman/v6/pkg/bindings"
	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/models"
)

// ConnectionManager manages SSH connections to machines
type ConnectionManager struct {
	cache           *ConnectionCache
	machineRepo     *models.MachineRepository
	encryption      *encryption.EncryptionService
	maxConcurrent   int
	queueTimeout    time.Duration
	connectionQueue chan connectionRequest
	mu              sync.Mutex
	activeConnections map[string]bool
}



type connectionRequest struct {
	machineID string
	userID    string
	response  chan<- connectionResponse
}

type connectionResponse struct {
	conn *bindings.Connection
	err  error
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
		cache:           cache,
		machineRepo:     machineRepo,
		encryption:      encryption,
		maxConcurrent:   maxConcurrent,
		queueTimeout:    queueTimeout,
		connectionQueue: make(chan connectionRequest, maxConcurrent),
		activeConnections: make(map[string]bool),
	}
}

// GetConnection gets a connection to a machine
func (m *ConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (*bindings.Connection, error) {
	// First try to get from cache
	conn, err := m.cache.GetOrCreate(ctx, machineID, userID, m.machineRepo, m.encryption)
	if err == nil {
		return conn, nil
	}

	// If cache is full or other issue, queue the request
	return m.QueueRequest(ctx, machineID, userID)
}

// QueueRequest queues a connection request
func (m *ConnectionManager) QueueRequest(ctx context.Context, machineID, userID string) (*bindings.Connection, error) {
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
		return response.conn, nil
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

// processQueue processes connection requests from the queue
func (m *ConnectionManager) processQueue() {
	for req := range m.connectionQueue {
		conn, err := m.cache.GetOrCreate(context.Background(), req.machineID, req.userID, m.machineRepo, m.encryption)
		
		// Send response
		response := connectionResponse{
			conn: conn,
			err:  err,
		}
		
		// Try to send response, but don't block if receiver is gone
		select {
		case req.response <- response:
			// Response sent successfully
		default:
			// Receiver is not listening anymore
			if conn != nil {
				// Note: bindings.Connection doesn't have Close method
				// Connection cleanup is handled automatically by Go's HTTP client
			}
		}
	}
}


