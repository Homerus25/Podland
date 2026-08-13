package connections

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/models"
)

// TestConnectionManager_Stop_TerminatesProcessQueueGoroutine verifies that
// Stop signals the processQueue goroutine to exit and waits for it.
//
// Background: internal/app.App.Close() needs a deterministic way to stop the
// connection manager's background goroutine so the in-process test harness
// (080-01) doesn't leak goroutines between subtests. Before this method
// existed, Run() spawned a processQueue goroutine that ranged over an
// unbounded channel with no shutdown path.
func TestConnectionManager_Stop_TerminatesProcessQueueGoroutine(t *testing.T) {
	mgr := NewConnectionManager(
		&ConnectionCache{
			connections: make(map[string]*ConnectionEntry),
			factory:     &ConnectionFactory{},
		},
		&models.MachineRepository{},
		&encryption.EncryptionService{},
		1,
		1*time.Second,
		10*time.Second,
	)
	mgr.Run()

	// processQueue must be running: confirm stopped channel is not yet
	// closed by performing a non-blocking check. We can't observe the
	// goroutine directly, so we just assert Stop returns and that a
	// second Stop is the no-op/idempotent path (it must not panic).
	done := make(chan struct{})
	go func() {
		mgr.Stop()
		close(done)
	}()

	select {
	case <-done:
		// expected — Stop returned, processQueue has exited.
	case <-time.After(2 * time.Second):
		t.Fatal("ConnectionManager.Stop did not return within 2s; processQueue goroutine likely still running")
	}

	// Stop must be safe to call again (idempotent). App.Close() promises
	// "safe to call once" at the App layer; the underlying Stop must not
	// double-close channels and panic when called twice.
	assert.NotPanics(t, func() { mgr.Stop() })
}
