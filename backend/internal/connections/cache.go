package connections

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.podman.io/podman/v6/pkg/bindings"
)

// ConnectionCache manages cached connections to machines
type ConnectionCache struct {
	connections map[string]*ConnectionEntry
	maxSize     int
	mu          sync.RWMutex
	factory     *ConnectionFactory
}

// ConnectionEntry represents a cached connection with its metadata
type ConnectionEntry struct {
	connCtx   context.Context
	conn      *bindings.Connection
	userID    string
	machineID string
	refCount  int
	lastUsed  time.Time
	isActive  bool
}

// NewConnectionCache creates a new connection cache
type ConnectionCacheOptions struct {
	MaxSize  int
	Factory  *ConnectionFactory
}

func NewConnectionCache(opts ConnectionCacheOptions) *ConnectionCache {
	cache := &ConnectionCache{
		connections: make(map[string]*ConnectionEntry),
		maxSize:     opts.MaxSize,
		factory:     opts.Factory,
	}
	go cache.cleanupLoop()
	return cache
}

// GetOrCreate gets an existing connection or creates a new one
func (c *ConnectionCache) GetOrCreate(
	ctx context.Context,
	machineID, userID string,
) (*bindings.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if connection already exists
	if entry, exists := c.connections[machineID]; exists {
		entry.refCount++
		entry.lastUsed = time.Now()
		entry.isActive = true
		return entry.conn, nil
	}

	// If cache is full, remove the least recently used idle connection
	if len(c.connections) >= c.maxSize {
		c.evictLRUIdle()
	}

	// Create new connection using factory
	conn, err := c.factory.CreateConnection(ctx, machineID, userID)
	if err != nil {
		return nil, err
	}

	// Store in cache
	entry := &ConnectionEntry{
		connCtx:   ctx, // Note: We might want to store the actual connection context
		conn:      conn,
		userID:    userID,
		machineID: machineID,
		refCount:  1,
		lastUsed:  time.Now(),
		isActive:  true,
	}
	c.connections[machineID] = entry

	return conn, nil
}

// GetOrCreateContext gets an existing connection context or creates a new one
func (c *ConnectionCache) GetOrCreateContext(
	ctx context.Context,
	machineID, userID string,
) (context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if connection already exists
	if entry, exists := c.connections[machineID]; exists {
		slog.Info("Machine Connection exists", slog.Any("machineID", machineID))
		entry.refCount++
		entry.lastUsed = time.Now()
		entry.isActive = true
		return entry.connCtx, nil
	}

	// If cache is full, remove the least recently used idle connection
	if len(c.connections) >= c.maxSize {
		c.evictLRUIdle()
	}

	// Create new connection context using factory
	connCtx, err := c.factory.CreateConnectionContext(ctx, machineID, userID)
	if err != nil {
		return nil, err
	}

	// Get the actual connection from the context for caching
	conn, err := bindings.GetClient(connCtx)
	if err != nil {
		return nil, err
	}

	// Store in cache
	entry := &ConnectionEntry{
		connCtx:   connCtx,
		conn:      conn,
		userID:    userID,
		machineID: machineID,
		refCount:  1,
		lastUsed:  time.Now(),
		isActive:  true,
	}
	c.connections[machineID] = entry

	return connCtx, nil
}

// Release decrements the reference count for a connection
func (c *ConnectionCache) Release(machineID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.connections[machineID]; exists {
		entry.refCount--
		entry.lastUsed = time.Now()
		if entry.refCount <= 0 {
			entry.isActive = false
		}
	}
}

// MarkActive marks a connection as active
func (c *ConnectionCache) MarkActive(machineID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.connections[machineID]; exists {
		entry.isActive = true
		entry.lastUsed = time.Now()
	}
}

// MarkIdle marks a connection as idle
func (c *ConnectionCache) MarkIdle(machineID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.connections[machineID]; exists {
		entry.isActive = false
		entry.lastUsed = time.Now()
	}
}

// evictLRUIdle removes the least recently used idle connection
func (c *ConnectionCache) evictLRUIdle() {
	var lruMachineID string
	var lruTime time.Time

	for machineID, entry := range c.connections {
		if !entry.isActive && (lruMachineID == "" || entry.lastUsed.Before(lruTime)) {
			lruMachineID = machineID
			lruTime = entry.lastUsed
		}
	}

	if lruMachineID != "" {
		// Close the connection
		if entry := c.connections[lruMachineID]; entry != nil && entry.conn != nil {
			// Note: bindings.Connection doesn't have Close method
			// Connection cleanup is handled automatically by Go's HTTP client
		}
		delete(c.connections, lruMachineID)
	}
}

// cleanupLoop periodically cleans up idle connections
func (c *ConnectionCache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes idle connections that haven't been used for a while
func (c *ConnectionCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	idleTimeout := 15 * time.Minute

	for machineID, entry := range c.connections {
		if !entry.isActive && now.Sub(entry.lastUsed) > idleTimeout {
			// Clean up the connection entry
			if entry.conn != nil {
				// Note: bindings.Connection doesn't have Close method
				// Connection cleanup is handled automatically by Go's HTTP client
			}
			delete(c.connections, machineID)
		}
	}
}
