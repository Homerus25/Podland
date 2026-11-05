package sshexec

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// --- fakes for the dialer-level seams ---

// fakeSSHClient is a test-only sshClient whose Close calls are counted. The
// cache tests never call NewSession — Run is exercised via fakeRunner in
// the Service-level tests instead — so NewSession returns an error.
type fakeSSHClient struct {
	closes atomic.Int32
}

func (c *fakeSSHClient) Close() error {
	c.closes.Add(1)
	return nil
}

func (c *fakeSSHClient) NewSession() (*ssh.Session, error) {
	return nil, errors.New("fakeSSHClient.NewSession not used in cache tests")
}

// fakeHostRepo is a minimal hostRepo for dialSSHProd tests.
type fakeHostRepo struct {
	hosts map[string]*models.Host
	err   error
}

func (r *fakeHostRepo) GetByID(id string) (*models.Host, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.hosts[id], nil
}

// fakeEncryption is a minimal encryptionSvc for dialSSHProd tests.
type fakeEncryption struct {
	decryptErr error
	// rawKey is returned by Decrypt when decryptErr is nil.
	rawKey string
}

func (e *fakeEncryption) Decrypt(_ []byte) (string, error) {
	if e.decryptErr != nil {
		return "", e.decryptErr
	}
	return e.rawKey, nil
}

// newTestDialer builds an sshDialer with a fake dialSSH that returns
// fakeSSHClients without touching the network. Cleanup goroutine is
// disabled (cleanupInterval=0). Each test can then assert against
// d.dialCount directly.
func newTestDialer(maxSize int) *sshDialer {
	return newSSHDialer(sshDialerConfig{
		maxSize:         maxSize,
		idleTimeout:     15 * time.Minute,
		cleanupInterval: 0, // no cleanup goroutine for tests
		connectTimeout:  30 * time.Second,
	})
}

// newRecordingDialer builds an sshDialer whose fake dialSSH hands out one
// fakeSSHClient per hostID, recording them for close-count assertions.
func newRecordingDialer(maxSize int) (*sshDialer, map[string]*fakeSSHClient) {
	return newRecordingDialerWithCfg(maxSize, 15*time.Minute, 0)
}

// newRecordingDialerWithCfg is newRecordingDialer with explicit cache
// timeouts — for cleanup tests that need a live cleanup goroutine.
func newRecordingDialerWithCfg(maxSize int, idleTimeout, cleanupInterval time.Duration) (*sshDialer, map[string]*fakeSSHClient) {
	clients := make(map[string]*fakeSSHClient)
	d := newSSHDialer(sshDialerConfig{
		maxSize:         maxSize,
		idleTimeout:     idleTimeout,
		cleanupInterval: cleanupInterval,
		connectTimeout:  30 * time.Second,
	})
	d.dialSSH = func(_ context.Context, hostID string) (sshClient, error) {
		c := &fakeSSHClient{}
		clients[hostID] = c
		return c, nil
	}
	return d, clients
}

// --- sshDialer.Dial cache behaviour (re-pinned against the unified semantics) ---
//
// The cache lifecycle itself is pinned once in internal/rescache's suite;
// these tests pin the adapter's projection — that the dialer runs over the
// resource cache with its dialer interface unchanged — and the behaviour
// changes the migration lands: no cross-host dial serialization, no
// mid-exec closes from eviction or cleanup, concurrent cold dials share one
// dial, the soft cap.

func TestSSHDialer_Dial_CachesClientPerHost(t *testing.T) {
	d := newTestDialer(10)
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		return &fakeSSHClient{}, nil
	}

	// First call: cache miss, dial happens.
	r1, err := d.Dial(context.Background(), "host-1")
	assert.NoError(t, err)
	assert.NotNil(t, r1)
	assert.Equal(t, int32(1), d.dialCount.Load(), "first Dial should produce one real dial")
	r1.Close()

	// Second call: cache hit, no new dial.
	r2, err := d.Dial(context.Background(), "host-1")
	assert.NoError(t, err)
	assert.NotNil(t, r2)
	assert.Equal(t, int32(1), d.dialCount.Load(), "second Dial for same host must reuse the cached client")
	r2.Close()

	// Different host: another dial.
	r3, err := d.Dial(context.Background(), "host-2")
	assert.NoError(t, err)
	assert.Equal(t, int32(2), d.dialCount.Load(), "different hostID must trigger a new dial")
	r3.Close()
}

func TestSSHDialer_Dial_PropagatesDialErrorAndDoesNotCache(t *testing.T) {
	d := newTestDialer(10)
	wantErr := errors.New("connection refused")
	var calls atomic.Int32
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		if calls.Add(1) == 1 {
			return nil, wantErr
		}
		return &fakeSSHClient{}, nil
	}

	_, err := d.Dial(context.Background(), "host-1")
	assert.ErrorIs(t, err, wantErr, "dialSSH error must propagate unwrapped")

	// The next call must try again (no cached failure): the second dial
	// succeeds and is the one cached dial.
	r, err := d.Dial(context.Background(), "host-1")
	assert.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, int32(1), d.dialCount.Load(), "the successful re-dial is the one cached dial")
	r.Close()
}

func TestSSHDialer_Dial_ConcurrentColdDialsShareOneDial(t *testing.T) {
	d := newTestDialer(10)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		started <- struct{}{}
		<-release
		return &fakeSSHClient{}, nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			r, err := d.Dial(context.Background(), "host-1")
			if err != nil {
				errs <- err
				return
			}
			r.Close()
			errs <- nil
		})
	}

	<-started // the cold dial is in flight, its pending entry already in the map
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.NoError(t, <-errs)
	assert.NoError(t, <-errs)
	assert.Equal(t, int32(1), d.dialCount.Load(),
		"concurrent cold dials for one host must share one dial")
}

func TestSSHDialer_Dial_ColdDialsToDifferentHostsDoNotSerialize(t *testing.T) {
	// The behaviour change this migration lands: the dial runs outside the
	// cache lock, so a cold dial to one host never blocks dials to
	// other hosts (the old dialer held its mutex across the dial).
	d := newTestDialer(10)
	blockA := make(chan struct{})
	startedA := make(chan struct{}, 1)
	startedB := make(chan struct{}, 1)
	d.dialSSH = func(_ context.Context, hostID string) (sshClient, error) {
		switch hostID {
		case "host-a":
			startedA <- struct{}{}
			<-blockA
		case "host-b":
			startedB <- struct{}{}
		}
		return &fakeSSHClient{}, nil
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		r, _ := d.Dial(context.Background(), "host-a")
		r.Close()
	})
	<-startedA
	wg.Go(func() {
		r, _ := d.Dial(context.Background(), "host-b")
		r.Close()
	})

	select {
	case <-startedB:
		// host-b's dial started while host-a's is still blocked:
		// cold dials to different hosts don't serialize behind one lock.
	case <-time.After(2 * time.Second):
		close(blockA)
		wg.Wait()
		t.Fatal("cold dial to a second host serialized behind the first host's in-flight dial")
	}

	close(blockA)
	wg.Wait()
}

func TestSSHDialer_Dial_LRUEvictionClosesLRUIdleClientWhenFull(t *testing.T) {
	d, clients := newRecordingDialer(2)

	r1, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	r1.Close() // m1 idle
	// Make m1 the oldest by sleeping briefly so m2's lastUsed is strictly newer.
	time.Sleep(2 * time.Millisecond)
	r2, err := d.Dial(context.Background(), "m2")
	assert.NoError(t, err)
	r2.Close() // m2 idle; m1 is now the LRU idle entry

	// Third distinct host: triggers LRU eviction. m1 (oldest idle) must go.
	r3, err := d.Dial(context.Background(), "m3")
	assert.NoError(t, err)
	defer r3.Close()

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"LRU eviction at the cap must close the least recently used idle client")
	assert.Zero(t, clients["m2"].closes.Load(), "the newer idle client must survive eviction")
	assert.Zero(t, clients["m3"].closes.Load(), "the freshly dialed client must not be closed")
}

func TestSSHDialer_Dial_EvictionNeverClosesInFlightClient(t *testing.T) {
	// The soft cap: when every entry is in flight, nothing is evicted and
	// the dial proceeds. An in-flight client is never closed by eviction.
	d, clients := newRecordingDialer(1)

	r1, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	// m1 stays in flight (r1 not closed): the cache is full and every
	// entry is in flight, so nothing is evicted and the dial proceeds.

	r2, err := d.Dial(context.Background(), "m2")
	assert.NoError(t, err)
	defer r2.Close()

	assert.Zero(t, clients["m1"].closes.Load(),
		"an in-flight client must never be closed by eviction, even past the soft cap")
	assert.Equal(t, int32(2), d.dialCount.Load(),
		"the dial proceeds past the soft cap when nothing is evictable")
	r1.Close()
}

// --- Invalidate: fail-fast ---

func TestSSHDialer_Invalidate_DropsEntryAndClosesClient(t *testing.T) {
	d, clients := newRecordingDialer(10)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	defer r.Close()

	d.Invalidate("m1")

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"Invalidate must Close() the dropped client")

	// Invalidate on a missing entry is a no-op (no panic).
	d.Invalidate("never-existed")
}

func TestSSHDialer_Invalidate_ClosesEvenMidFlight(t *testing.T) {
	// Fail-fast: the client closes now, even while an exec holds it; the
	// affected call surfaces the close as its own session failure.
	d, clients := newRecordingDialer(10)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	// m1 is in flight (r not closed).

	d.Invalidate("m1")

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"Invalidate must close the client even mid-flight (fail-fast)")

	r.Close()
	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"releasing after Invalidate must not close the client twice")
}

// --- RemoveHost: graceful invalidation (mirrors connections.Manager) ---

func TestSSHDialer_RemoveHost_WaitsForInFlightRunnersBeforeClosing(t *testing.T) {
	// The graceful-removal contract (docs/architecture-decisions.md §4): in-flight execs run to
	// completion; the client closes when the LAST in-flight exec releases.
	d, clients := newRecordingDialer(10)
	ctx := context.Background()

	r1, err := d.Dial(ctx, "m1") // in-flight exec #1
	assert.NoError(t, err)
	r2, err := d.Dial(ctx, "m1") // in-flight exec #2 (same cached client)
	assert.NoError(t, err)

	d.RemoveHost("m1")

	assert.Equal(t, int32(0), clients["m1"].closes.Load(),
		"in-flight execs must keep the client open after removal")

	r1.Close()
	assert.Equal(t, int32(0), clients["m1"].closes.Load(),
		"client must stay open while another exec is still in flight")

	r2.Close()
	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"client must close exactly when the last in-flight exec releases")
}

func TestSSHDialer_RemoveHost_ClosesIdleClientImmediately(t *testing.T) {
	d, clients := newRecordingDialer(10)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	r.Close() // exec finished before removal: the entry is idle

	d.RemoveHost("m1")

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"an idle entry's client must close immediately on removal")
}

func TestSSHDialer_RemoveHost_UnknownHostIsNoOp(t *testing.T) {
	d, _ := newRecordingDialer(10)

	assert.NotPanics(t, func() { d.RemoveHost("no-such-host") })
}

func TestSSHDialer_RemoveHost_LaterExecDialsAFreshClient(t *testing.T) {
	// No tombstone: after removal, the next exec for the host dials
	// again. The fresh client must be live, not the removed one.
	d, clients := newRecordingDialer(10)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	r.Close()

	d.RemoveHost("m1")
	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"removal of an idle client closes it immediately")

	oldClient := clients["m1"]
	r2, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	assert.Equal(t, int32(2), d.dialCount.Load(), "post-removal Dial must dial again")
	assert.NotSame(t, oldClient, clients["m1"],
		"the fresh dial's client must be cached, not the removed one")

	r2.Close()
	assert.Equal(t, int32(1), oldClient.closes.Load(),
		"releasing the fresh runner must not close the removed host's old client")
	assert.Zero(t, clients["m1"].closes.Load(),
		"the fresh client must not be closed by releasing its own runner")
}

func TestSSHDialer_RemoveHost_IsIdempotent(t *testing.T) {
	d, clients := newRecordingDialer(10)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	r.Close()

	d.RemoveHost("m1")
	d.RemoveHost("m1")

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"repeat removal must not close the client twice")
}

// --- idle cleanup (re-pinned: skips in-use clients; never closes mid-exec) ---

func TestSSHDialer_Cleanup_ClosesIdleClientPastIdleTimeout(t *testing.T) {
	d, clients := newRecordingDialerWithCfg(10, 20*time.Millisecond, 10*time.Millisecond)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	r.Close() // m1 idle

	require.Eventually(t, func() bool { return clients["m1"].closes.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"idle cleanup must close a client past the idle timeout")
}

func TestSSHDialer_Cleanup_KeepsRecentlyUsedClient(t *testing.T) {
	d, clients := newRecordingDialerWithCfg(10, time.Hour, 10*time.Millisecond)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	defer r.Close()

	// The client is well within the idle timeout: cleanup must not close it.
	require.Never(t, func() bool { return clients["m1"].closes.Load() == 1 },
		100*time.Millisecond, 10*time.Millisecond,
		"recently-used clients must survive cleanup")
}

func TestSSHDialer_Cleanup_SkipsInUseClient(t *testing.T) {
	// The behaviour change this migration lands: idle cleanup never closes
	// a client mid-exec (the old dialer closed clients past the timeout even
	// while an exec was running).
	d, clients := newRecordingDialerWithCfg(10, 20*time.Millisecond, 10*time.Millisecond)

	r, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	// m1 stays in flight well past the idle timeout.

	require.Never(t, func() bool { return clients["m1"].closes.Load() == 1 },
		100*time.Millisecond, 10*time.Millisecond,
		"idle cleanup must never close a client mid-exec, even past the timeout")

	r.Close()
	require.Eventually(t, func() bool { return clients["m1"].closes.Load() == 1 },
		2*time.Second, 5*time.Millisecond,
		"once released, the now-idle client is closed by cleanup")
}

// --- Close: stops the goroutine and drains all clients ---

// TestSSHDialer_Close_BlocksUntilCleanupGoroutineExits verifies the
// blocking-until-exit contract on Close. ConnectionManager.Stop blocks on
// the cache's stopped channel for the same reason; the symmetry is what lets
// internal/app.App.Close guarantee "no goroutine is left running" by the
// time it returns.
func TestSSHDialer_Close_BlocksUntilCleanupGoroutineExits(t *testing.T) {
	d := newSSHDialer(sshDialerConfig{
		maxSize:         10,
		cleanupInterval: 10 * time.Millisecond,
	})

	exited := make(chan struct{})
	go func() {
		_ = d.Close()
		close(exited)
	}()

	select {
	case <-exited:
		// expected: Close returned promptly once the cleanup goroutine exited
	case <-time.After(2 * time.Second):
		t.Fatal("sshDialer.Close did not return within 2s; cleanupLoop goroutine likely still running")
	}

	// After Close returns, the cleanup goroutine must be gone — prove it
	// by asserting Close is idempotent.
	assert.NotPanics(t, func() { _ = d.Close() })
}

func TestSSHDialer_Close_DrainsAllClients(t *testing.T) {
	d, clients := newRecordingDialer(10)

	r1, err := d.Dial(context.Background(), "m1")
	assert.NoError(t, err)
	defer r1.Close()
	r2, err := d.Dial(context.Background(), "m2")
	assert.NoError(t, err)
	defer r2.Close()

	require.NoError(t, d.Close())

	assert.Equal(t, int32(1), clients["m1"].closes.Load(),
		"Close must drain (close) every cached client")
	assert.Equal(t, int32(1), clients["m2"].closes.Load(),
		"Close must drain (close) every cached client")
}

// --- dialSSHProd error paths (no live SSH server needed) ---

func TestSSHDialer_dialSSHProd_HostNotFound(t *testing.T) {
	d := newSSHDialer(sshDialerConfig{
		hostRepo:   &fakeHostRepo{hosts: map[string]*models.Host{}},
		encryption: &fakeEncryption{},
	})

	_, err := d.dialSSHProd(context.Background(), "missing")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host not found")
}

func TestSSHDialer_dialSSHProd_RepoErrorWrapped(t *testing.T) {
	repoErr := errors.New("disk i/o")
	d := newSSHDialer(sshDialerConfig{
		hostRepo:   &fakeHostRepo{err: repoErr},
		encryption: &fakeEncryption{},
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch host")
	assert.ErrorIs(t, err, repoErr, "the underlying repo error must be wrapped, not swallowed")
}

func TestSSHDialer_dialSSHProd_DecryptFailureWrapped(t *testing.T) {
	host := &models.Host{
		ID:              "m1",
		Hostname:        "10.0.0.1",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("garbage"),
	}
	decErr := errors.New("bad ciphertext")
	d := newSSHDialer(sshDialerConfig{
		hostRepo:   &fakeHostRepo{hosts: map[string]*models.Host{"m1": host}},
		encryption: &fakeEncryption{decryptErr: decErr},
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt SSH key")
	assert.ErrorIs(t, err, decErr)
}

func TestSSHDialer_dialSSHProd_InvalidPEMWrapped(t *testing.T) {
	// fakeEncryption returns the literal "raw-key" — not a PEM — so
	// ssh.ParsePrivateKey must fail with a parse error that we wrap.
	host := &models.Host{
		ID:              "m1",
		Hostname:        "10.0.0.1",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("whatever"),
	}
	d := newSSHDialer(sshDialerConfig{
		hostRepo:   &fakeHostRepo{hosts: map[string]*models.Host{"m1": host}},
		encryption: &fakeEncryption{rawKey: "raw-key"},
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse SSH key")
}

// TestSSHDialer_dialSSHProd_UnreachableHost verifies that a TCP-dial failure
// (DNS lookup failure for a reserved-nonexistent domain) is wrapped with the
// "failed to dial" prefix. We use RFC-6761's `.invalid` TLD, which DNS
// resolvers are required to never resolve, so this fails fast without
// depending on any specific port being closed.
func TestSSHDialer_dialSSHProd_UnreachableHost(t *testing.T) {
	host := &models.Host{
		ID:              "m1",
		Hostname:        "nonexistent.invalid",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("ignored"),
	}
	d := newSSHDialer(sshDialerConfig{
		hostRepo:       &fakeHostRepo{hosts: map[string]*models.Host{"m1": host}},
		encryption:     &fakeEncryption{rawKey: validPEM(t)},
		connectTimeout: 2 * time.Second,
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to dial")
}

// validPEM returns a real PEM-encoded ECDSA private key for tests that need
// to get past ssh.ParsePrivateKey. Generated fresh each call so no static
// key material lives in the repo.
func validPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

// --- classifyRunResult (the pure exit-code classifier) ---
//
// These tests pin the precedence rules: ctx cancellation wins; nil runErr is
// exit 0; *ssh.ExitError is a non-zero exit surfaced as a result, not an
// error; anything else is a real failure. Testing this directly means we
// don't need a live SSH server to validate the contract.
//
// We can't easily construct a real *ssh.ExitError (its Waitmsg is unexported
// and only populated by the SSH protocol layer), so the *ssh.ExitError branch
// is exercised indirectly via the Service-level tests (fakeRunner returns
// whatever ExecResult we configure, bypassing the classifier).

func TestClassifyRunResult_SuccessIsExitZero(t *testing.T) {
	res, err := classifyRunResult(nil, nil, "ok\n", "")

	assert.NoError(t, err)
	assert.Equal(t, ExecResult{Stdout: "ok\n", Stderr: "", ExitCode: 0}, res)
}

func TestClassifyRunResult_RealErrorIsReturned(t *testing.T) {
	original := errors.New("connection reset")
	res, err := classifyRunResult(original, nil, "partial", "")

	assert.Error(t, err)
	assert.ErrorIs(t, err, original, "real runErr must be wrapped, not swallowed")
	assert.Contains(t, err.Error(), "sshexec: command failed")
	assert.Equal(t, ExecResult{}, res,
		"real failure must NOT populate result fields")
}

func TestClassifyRunResult_ContextCancellationBeatsExit(t *testing.T) {
	// Even if runErr is also set, ctxErr must take precedence — the
	// Service contract requires Exec to surface ctx.Err() so callers can
	// detect abandonment.
	runErr := errors.New("session killed")
	res, err := classifyRunResult(runErr, context.Canceled, "ignored", "ignored")

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, ExecResult{}, res)
}

func TestClassifyRunResult_ContextCancellationBeatsSuccess(t *testing.T) {
	// Even a clean-looking success must be overridden when ctx was
	// cancelled — the partial output must not be trusted.
	res, err := classifyRunResult(nil, context.DeadlineExceeded, "partial", "")

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, ExecResult{}, res)
}
