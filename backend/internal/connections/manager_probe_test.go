package connections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/encryption"
	"github.com/Homerus25/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/bindings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newProbeCache returns a cache with the given connection contexts
// pre-seeded per machine, standing in for SSH-established tunnels so the
// probe exercises the "cached connection" path without a real host.
func newProbeCache(t *testing.T, connCtxByMachine map[string]context.Context) *ConnectionCache {
	t.Helper()
	connections := make(map[string]*ConnectionEntry, len(connCtxByMachine))
	for machineID, connCtx := range connCtxByMachine {
		conn, err := bindings.GetClient(connCtx)
		require.NoError(t, err)
		connections[machineID] = &ConnectionEntry{
			connCtx:   connCtx,
			conn:      conn,
			machineID: machineID,
			refCount:  1,
			isActive:  true,
		}
	}
	return &ConnectionCache{connections: connections}
}

// pingServer starts an httptest server that answers /_ping with the given
// status code, and returns its tcp:// URI suitable for bindings.
func pingServer(t *testing.T, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return "tcp://" + server.Listener.Addr().String()
}

func tunnelCtx(t *testing.T, uri string) context.Context {
	t.Helper()
	connCtx, err := bindings.NewConnectionWithOptions(context.Background(), bindings.Options{URI: uri})
	require.NoError(t, err)
	return connCtx
}

func newProbeManager(cache *ConnectionCache, testTimeout time.Duration) *ConnectionManager {
	return NewConnectionManager(cache, &models.MachineRepository{}, &encryption.EncryptionService{}, 1, 1*time.Second, testTimeout)
}

func TestConnectionManager_ProbeMachine(t *testing.T) {
	t.Run("ping succeeds over cached connection", func(t *testing.T) {
		connCtx := tunnelCtx(t, pingServer(t, http.StatusOK))
		mgr := newProbeManager(newProbeCache(t, map[string]context.Context{"machine-1": connCtx}), 10*time.Second)

		err := mgr.ProbeMachine("machine-1", "user-1")

		assert.NoError(t, err, "SSH reachable + ping OK must mean online (nil)")
	})

	t.Run("ping failure over cached connection is an error", func(t *testing.T) {
		// The bindings layer pings at connect time, so the tunnel must be
		// established against a healthy Podman API first; the server then
		// starts answering 500, simulating the Podman service going away
		// while the SSH connection stays cached.
		var status atomic.Int32
		status.Store(http.StatusOK)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(int(status.Load()))
		}))
		t.Cleanup(server.Close)
		connCtx := tunnelCtx(t, "tcp://"+server.Listener.Addr().String())
		status.Store(http.StatusInternalServerError)
		mgr := newProbeManager(newProbeCache(t, map[string]context.Context{"machine-1": connCtx}), 10*time.Second)

		err := mgr.ProbeMachine("machine-1", "user-1")

		assert.Error(t, err, "non-200 ping must mean offline (error)")
	})

	t.Run("probe is bounded by the test timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)
		connCtx := tunnelCtx(t, "tcp://"+server.Listener.Addr().String())
		mgr := newProbeManager(newProbeCache(t, map[string]context.Context{"machine-1": connCtx}), 20*time.Millisecond)

		err := mgr.ProbeMachine("machine-1", "user-1")

		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"probe exceeding testTimeout must return the standard deadline error (→ offline)")
	})
}
