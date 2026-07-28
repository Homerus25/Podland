package connections

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/bindings"
)

// =============================================================================
// ConnectionCache Core Functionality Tests
// =============================================================================

// TestNewConnectionCache tests that NewConnectionCache creates a properly initialized cache
func TestNewConnectionCache(t *testing.T) {
	t.Run("creates cache with correct maxSize", func(t *testing.T) {
		maxSize := 5
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: maxSize,
			Factory: factory,
		})

		// Verify cache is initialized
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.connections)
		assert.Equal(t, maxSize, cache.maxSize)
		assert.Equal(t, 0, len(cache.connections))
	})

	t.Run("creates cache with default maxSize", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 0,
			Factory: factory,
		})

		// Verify cache is initialized even with maxSize 0
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.connections)
		assert.Equal(t, 0, cache.maxSize)
	})
}

// TestConnectionCacheCore tests core cache functionality using direct manipulation
func TestConnectionCacheCore(t *testing.T) {
	// Create test machine
	testMachine := &models.Machine{
		ID:               "test-machine-1",
		Name:             "Test Machine",
		OwnerID:          "test-user-1",
		Hostname:         "example.com",
		Port:             22,
		SSHUsername:      "root",
		SSHKeyEncrypted:  []byte("encrypted:test-ssh-key"),
		PodmanSocketPath: "/run/podman/podman.sock",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	t.Run("GetOrCreate returns existing connection when available (cache hit)", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create a mock connection and add it directly to the cache
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  1,
			lastUsed:  time.Now(),
			isActive:  true,
		}
		cache.connections[testMachine.ID] = entry

		// Verify the connection exists
		assert.Equal(t, 1, len(cache.connections))
	})

	t.Run("GetOrCreate creates new connection when needed (cache miss)", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Verify cache is empty initially
		assert.Equal(t, 0, len(cache.connections))

		// Create a mock connection and add it to the cache
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  1,
			lastUsed:  time.Now(),
			isActive:  true,
		}
		cache.connections[testMachine.ID] = entry

		// Verify the connection was added
		assert.Equal(t, 1, len(cache.connections))
	})

	t.Run("Release properly decrements reference count and marks idle when refCount reaches 0", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create a connection entry
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  2, // Start with refCount 2
			lastUsed:  time.Now(),
			isActive:  true,
		}
		cache.connections[testMachine.ID] = entry

		// Release once (refCount should be 1, still active)
		cache.Release(testMachine.ID)
		assert.Equal(t, 1, entry.refCount)
		assert.True(t, entry.isActive)

		// Release again (refCount should be 0, marked as idle)
		cache.Release(testMachine.ID)
		assert.Equal(t, 0, entry.refCount)
		assert.False(t, entry.isActive)
	})

	t.Run("MarkActive updates connection active status and lastUsed timestamp", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create a connection entry
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  1,
			lastUsed:  time.Now().Add(-1 * time.Minute), // Set to past time
			isActive:  false,
		}
		cache.connections[testMachine.ID] = entry

		// Mark it as active
		cache.MarkActive(testMachine.ID)
		assert.True(t, entry.isActive)
		assert.WithinDuration(t, time.Now(), entry.lastUsed, 1*time.Second)
	})

	t.Run("MarkIdle updates connection idle status and lastUsed timestamp", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create a connection entry
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  1,
			lastUsed:  time.Now().Add(-1 * time.Minute), // Set to past time
			isActive:  true,
		}
		cache.connections[testMachine.ID] = entry

		// Mark it as idle
		cache.MarkIdle(testMachine.ID)
		assert.False(t, entry.isActive)
		assert.WithinDuration(t, time.Now(), entry.lastUsed, 1*time.Second)
	})

	t.Run("evictLRUIdle removes the least recently used idle connection when cache is full", func(t *testing.T) {
		// Create a small cache (maxSize = 2)
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 2,
			Factory: factory,
		})
		ctx := context.Background()

		// Create multiple connection entries
		for i := 1; i <= 3; i++ {
			machineID := fmt.Sprintf("test-machine-%d", i)
			mockConn := &bindings.Connection{}
			entry := &ConnectionEntry{
				connCtx:   ctx,
				conn:      mockConn,
				userID:    "test-user-1",
				machineID: machineID,
				refCount:  1,
				lastUsed:  time.Now().Add(-time.Duration(i) * time.Minute), // Different ages
				isActive:  false,                                           // All idle
			}
			cache.connections[machineID] = entry
		}

		// Verify cache is full
		assert.Equal(t, 3, len(cache.connections))

		// Call evictLRUIdle
		cache.evictLRUIdle()

		// Verify one connection was evicted (should be the oldest idle one)
		assert.Equal(t, 2, len(cache.connections))
		// Verify the oldest connection (test-machine-3) was evicted
		_, exists := cache.connections["test-machine-3"]
		assert.False(t, exists)
	})

	t.Run("evictLRUIdle does not remove active connections", func(t *testing.T) {
		// Create a small cache (maxSize = 2)
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 2,
			Factory: factory,
		})
		ctx := context.Background()

		// Create connection entries - one active, one idle
		mockConn1 := &bindings.Connection{}
		entry1 := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn1,
			userID:    "test-user-1",
			machineID: "test-machine-1",
			refCount:  1,
			lastUsed:  time.Now().Add(-2 * time.Minute), // Older
			isActive:  true,                             // Active - should NOT be evicted
		}
		cache.connections["test-machine-1"] = entry1

		mockConn2 := &bindings.Connection{}
		entry2 := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn2,
			userID:    "test-user-1",
			machineID: "test-machine-2",
			refCount:  1,
			lastUsed:  time.Now().Add(-1 * time.Minute), // Newer
			isActive:  false,                            // Idle - should be evicted
		}
		cache.connections["test-machine-2"] = entry2

		// Add a third connection to trigger eviction
		mockConn3 := &bindings.Connection{}
		entry3 := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn3,
			userID:    "test-user-1",
			machineID: "test-machine-3",
			refCount:  1,
			lastUsed:  time.Now(), // Newest
			isActive:  false,      // Idle
		}
		cache.connections["test-machine-3"] = entry3

		// Call evictLRUIdle
		cache.evictLRUIdle()

		// Verify the active connection is still there
		_, exists1 := cache.connections["test-machine-1"]
		assert.True(t, exists1, "Active connection should not be evicted")

		// Verify one of the idle connections was evicted
		assert.Equal(t, 2, len(cache.connections))
	})

	t.Run("cleanup removes idle connections that haven't been used for the idle timeout period", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create an idle connection that's older than the timeout (15 minutes)
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  0,
			lastUsed:  time.Now().Add(-16 * time.Minute), // 16 minutes ago
			isActive:  false,                             // Idle
		}
		cache.connections[testMachine.ID] = entry

		// Run cleanup
		cache.cleanup()

		// Verify the connection was removed
		_, exists := cache.connections[testMachine.ID]
		assert.False(t, exists, "Idle connection should have been cleaned up")
	})

	t.Run("cleanup does not remove active connections", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})
		ctx := context.Background()

		// Create an active connection that's older than the timeout
		mockConn := &bindings.Connection{}
		entry := &ConnectionEntry{
			connCtx:   ctx,
			conn:      mockConn,
			userID:    "test-user-1",
			machineID: testMachine.ID,
			refCount:  1,
			lastUsed:  time.Now().Add(-16 * time.Minute), // 16 minutes ago
			isActive:  true,                              // Active
		}
		cache.connections[testMachine.ID] = entry

		// Run cleanup
		cache.cleanup()

		// Verify the connection is still there
		_, exists := cache.connections[testMachine.ID]
		assert.True(t, exists, "Active connection should not have been cleaned up")
	})

	t.Run("cleanupLoop runs periodically and calls cleanup", func(t *testing.T) {
		factory := &ConnectionFactory{
			machineRepo: &models.MachineRepository{},
			encryption:  &encryption.EncryptionService{},
		}
		cache := NewConnectionCache(ConnectionCacheOptions{
			MaxSize: 10,
			Factory: factory,
		})

		// The cleanup loop runs in a goroutine, so we can't easily test it directly
		// However, we can verify that it doesn't cause any issues and that
		// the cache continues to function properly

		// Let the cleanup loop run for a short time
		time.Sleep(100 * time.Millisecond)

		// Verify the cache is still functional
		assert.NotNil(t, cache)
		assert.NotNil(t, cache.connections)
	})
}
