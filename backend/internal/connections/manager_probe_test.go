package connections

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/bindings"
	"golang.org/x/crypto/ssh"
)

// realDialMu serializes the tests' real SSH dials. The fork's sshClient
// writes ssh_config.DefaultUserSettings.IgnoreErrors without
// synchronization on every dial, so two dials without a happens-before
// edge between them trip the race detector. That is a pre-existing
// upstream race concurrent production dials can also hit; fixing it
// belongs in the fork, not in this package.
var realDialMu sync.Mutex

// newProbeManager returns a manager with live podman connection contexts
// pre-seeded per machine, standing in for SSH-established tunnels so the
// probe exercises the "cached connection" path without a real host. The
// cache is warmed by acquiring each pre-seeded machine once (its dial
// returns the pre-seeded context); the dial is then swapped for one that
// fails loudly, so a probe that ever tries to dial fails the test.
func newProbeManager(t *testing.T, connCtxByMachine map[string]context.Context, testTimeout time.Duration) *ConnectionManager {
	t.Helper()
	mgr := newManager(managerOptions{
		maxSize:         10,
		testTimeout:     testTimeout,
		idleTimeout:     15 * time.Minute,
		cleanupInterval: 0, // no cleanup goroutine: entries must survive the test
		dial: func(_ context.Context, machineID string) (context.Context, bindings.CloseFunc, error) {
			connCtx, ok := connCtxByMachine[machineID]
			if !ok {
				return nil, nil, errors.New("probe tests must not dial: connections are pre-seeded")
			}
			return connCtx, func() {}, nil
		},
	})
	for machineID, connCtx := range connCtxByMachine {
		if _, err := bindings.GetClient(connCtx); err != nil {
			t.Fatalf("pre-seeded context for %s: %v", machineID, err)
		}
		_, release, err := mgr.cache.Acquire(context.Background(), machineID)
		if err != nil {
			t.Fatalf("seed cache for %s: %v", machineID, err)
		}
		release()
	}
	// Seeding is done: from here on the dial must never be called again —
	// the probe exercises the cached-connection path. Swap in a failing
	// dial so a probe that dials fails loudly.
	mgr.dial = func(context.Context, string) (context.Context, bindings.CloseFunc, error) {
		return nil, nil, errors.New("probe tests must not dial: connections are pre-seeded")
	}
	return mgr
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
	realDialMu.Lock()
	defer realDialMu.Unlock()
	connCtx, err := bindings.NewConnectionWithOptions(context.Background(), bindings.Options{URI: uri})
	require.NoError(t, err)
	return connCtx
}

func TestConnectionManager_ProbeMachine(t *testing.T) {
	t.Run("ping succeeds over cached connection", func(t *testing.T) {
		connCtx := tunnelCtx(t, pingServer(t, http.StatusOK))
		mgr := newProbeManager(t, map[string]context.Context{"machine-1": connCtx}, 10*time.Second)

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
		mgr := newProbeManager(t, map[string]context.Context{"machine-1": connCtx}, 10*time.Second)

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
		mgr := newProbeManager(t, map[string]context.Context{"machine-1": connCtx}, 20*time.Millisecond)

		err := mgr.ProbeMachine("machine-1", "user-1")

		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"probe exceeding testTimeout must return the standard deadline error (→ offline)")
	})
}

// --- registration TestConnection: timeout semantics ---

// writeTestIdentity returns a valid ed25519 private key as PEM content —
// the raw-credential shape TestConnection takes (the SSH dial parses the
// identity BEFORE opening the TCP connection, so an invalid key would fail
// fast and never reach the handshake the timeout tests need to block on).
func writeTestIdentity(t *testing.T) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(key, "")
	require.NoError(t, err)
	return string(pem.EncodeToMemory(block))
}

// silentSSHServer starts a TCP listener that accepts connections but never
// sends the SSH version banner, so the client handshake blocks and only
// the manager's deadline race can decide the outcome.
func silentSSHServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestConnectionManager_TestConnection_BoundedByTestTimeout(t *testing.T) {
	port := silentSSHServer(t)
	identity := writeTestIdentity(t)
	mgr := newProbeManager(t, nil, 50*time.Millisecond)

	realDialMu.Lock()
	err := mgr.TestConnection(context.Background(), "127.0.0.1", port, "root", identity, "/run/podman/podman.sock")
	realDialMu.Unlock()

	// The timeout crosses as the typed TestTimeoutError carrying the
	// configured bound (issue 399): the registration flow names the
	// bound in its 400 message, so the bare context deadline — which
	// carries no bound — no longer crosses the package boundary.
	var timeoutErr *TestTimeoutError
	require.ErrorAs(t, err, &timeoutErr,
		"a silent host must surface the typed probe timeout (→ registration 400 timeout), not hang")
	assert.Equal(t, 50*time.Millisecond, timeoutErr.Bound,
		"the typed timeout must carry the configured bound it exceeded")
}

func TestConnectionManager_TestConnection_FastDialFailureIsNotATimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	identity := writeTestIdentity(t)
	mgr := newProbeManager(t, nil, 10*time.Second)

	realDialMu.Lock()
	err = mgr.TestConnection(context.Background(), "127.0.0.1", port, "root", identity, "/run/podman/podman.sock")
	realDialMu.Unlock()

	require.Error(t, err)
	var timeoutErr *TestTimeoutError
	assert.NotErrorAs(t, err, &timeoutErr,
		"connection refused is a real probe failure (→ registration 400), not the probe timeout")
}
