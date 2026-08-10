package sshexec

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourorg/podland/backend/internal/models"
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

// fakeMachineRepo is a minimal machineRepo for dialSSHProd tests.
type fakeMachineRepo struct {
	machines map[string]*models.Machine
	err      error
}

func (r *fakeMachineRepo) GetByID(id string) (*models.Machine, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.machines[id], nil
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
// d.dialCount and d.clients directly.
func newTestDialer(maxSize int) *sshDialer {
	return newSSHDialer(sshDialerConfig{
		maxSize:         maxSize,
		idleTimeout:     15 * time.Minute,
		cleanupInterval: 0, // no cleanup goroutine for tests
		connectTimeout:  30 * time.Second,
	})
}

// --- sshDialer.Dial cache behaviour ---

func TestSSHDialer_Dial_CachesClientPerMachine(t *testing.T) {
	d := newTestDialer(10)
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		return &fakeSSHClient{}, nil
	}

	// First call: cache miss, dial happens.
	r1, err := d.Dial(context.Background(), "machine-1")
	assert.NoError(t, err)
	assert.NotNil(t, r1)
	assert.Equal(t, 1, d.dialCount, "first Dial should produce one real dial")

	// Second call: cache hit, no new dial.
	r2, err := d.Dial(context.Background(), "machine-1")
	assert.NoError(t, err)
	assert.NotNil(t, r2)
	assert.Equal(t, 1, d.dialCount, "second Dial for same machine must reuse the cached client")

	// Different machine: another dial.
	_, err = d.Dial(context.Background(), "machine-2")
	assert.NoError(t, err)
	assert.Equal(t, 2, d.dialCount, "different machineID must trigger a new dial")
}

func TestSSHDialer_Dial_PropagatesDialErrorAndDoesNotCache(t *testing.T) {
	d := newTestDialer(10)
	wantErr := errors.New("connection refused")
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		return nil, wantErr
	}

	_, err := d.Dial(context.Background(), "machine-1")
	assert.ErrorIs(t, err, wantErr, "dialSSH error must propagate unwrapped")
	assert.Empty(t, d.clients, "a failed dial must leave the cache untouched")

	// And the next call must try again (no cached failure).
	_, _ = d.Dial(context.Background(), "machine-1")
	assert.Empty(t, d.clients)
}

func TestSSHDialer_Dial_LRUEvictionWhenFull(t *testing.T) {
	d := newTestDialer(2)
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		return &fakeSSHClient{}, nil
	}

	// Fill the cache.
	_, _ = d.Dial(context.Background(), "m1")
	// Make m1 the oldest by sleeping briefly so m2's lastUsed is strictly newer.
	time.Sleep(2 * time.Millisecond)
	_, _ = d.Dial(context.Background(), "m2")
	assert.Len(t, d.clients, 2)

	// Third distinct machine: triggers LRU eviction. m1 (oldest) must go.
	_, _ = d.Dial(context.Background(), "m3")

	assert.Len(t, d.clients, 2, "cache size must stay bounded at maxSize")
	assert.Contains(t, d.clients, "m2")
	assert.Contains(t, d.clients, "m3")
	assert.NotContains(t, d.clients, "m1", "LRU entry (m1) must be evicted")
}

func TestSSHDialer_Invalidate_DropsEntryAndClosesClient(t *testing.T) {
	d := newTestDialer(10)
	var inserted *fakeSSHClient
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		c := &fakeSSHClient{}
		inserted = c
		return c, nil
	}

	_, _ = d.Dial(context.Background(), "m1")
	assert.Len(t, d.clients, 1)

	d.Invalidate("m1")

	assert.Len(t, d.clients, 0, "Invalidate must drop the cached entry")
	assert.Equal(t, int32(1), inserted.closes.Load(),
		"Invalidate must Close() the dropped client")

	// Invalidate on a missing entry is a no-op (no panic).
	d.Invalidate("never-existed")
	assert.Len(t, d.clients, 0)
}

func TestSSHDialer_Cleanup_DropsIdleEntriesBeyondIdleTimeout(t *testing.T) {
	d := newTestDialer(10)
	var inserted *fakeSSHClient
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		c := &fakeSSHClient{}
		inserted = c
		return c, nil
	}

	_, _ = d.Dial(context.Background(), "m1")
	assert.Len(t, d.clients, 1)

	// Force the entry's lastUsed into the distant past and shrink the
	// idle timeout so cleanup will reap it.
	d.mu.Lock()
	d.idleTimeout = 1 * time.Nanosecond
	for _, e := range d.clients {
		e.lastUsed = time.Now().Add(-1 * time.Hour)
	}
	d.mu.Unlock()

	d.cleanup()

	assert.Len(t, d.clients, 0, "idle entry past IdleTimeout must be cleaned up")
	assert.Equal(t, int32(1), inserted.closes.Load(),
		"cleanup must Close() the dropped client")
}

func TestSSHDialer_Cleanup_KeepsRecentlyUsedEntries(t *testing.T) {
	d := newTestDialer(10)
	d.dialSSH = func(_ context.Context, _ string) (sshClient, error) {
		return &fakeSSHClient{}, nil
	}

	_, _ = d.Dial(context.Background(), "m1")

	d.mu.Lock()
	d.idleTimeout = 1 * time.Hour // entries are well within this
	d.mu.Unlock()

	d.cleanup()

	assert.Len(t, d.clients, 1, "recently-used entries must survive cleanup")
}

// --- dialSSHProd error paths (no live SSH server needed) ---

func TestSSHDialer_dialSSHProd_MachineNotFound(t *testing.T) {
	d := newSSHDialer(sshDialerConfig{
		machineRepo: &fakeMachineRepo{machines: map[string]*models.Machine{}},
		encryption:  &fakeEncryption{},
	})

	_, err := d.dialSSHProd(context.Background(), "missing")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "machine not found")
}

func TestSSHDialer_dialSSHProd_RepoErrorWrapped(t *testing.T) {
	repoErr := errors.New("disk i/o")
	d := newSSHDialer(sshDialerConfig{
		machineRepo: &fakeMachineRepo{err: repoErr},
		encryption:  &fakeEncryption{},
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch machine")
	assert.ErrorIs(t, err, repoErr, "the underlying repo error must be wrapped, not swallowed")
}

func TestSSHDialer_dialSSHProd_DecryptFailureWrapped(t *testing.T) {
	machine := &models.Machine{
		ID:              "m1",
		Hostname:        "10.0.0.1",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("garbage"),
	}
	decErr := errors.New("bad ciphertext")
	d := newSSHDialer(sshDialerConfig{
		machineRepo: &fakeMachineRepo{machines: map[string]*models.Machine{"m1": machine}},
		encryption:  &fakeEncryption{decryptErr: decErr},
	})

	_, err := d.dialSSHProd(context.Background(), "m1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt SSH key")
	assert.ErrorIs(t, err, decErr)
}

func TestSSHDialer_dialSSHProd_InvalidPEMWrapped(t *testing.T) {
	// fakeEncryption returns the literal "raw-key" — not a PEM — so
	// ssh.ParsePrivateKey must fail with a parse error that we wrap.
	machine := &models.Machine{
		ID:              "m1",
		Hostname:        "10.0.0.1",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("whatever"),
	}
	d := newSSHDialer(sshDialerConfig{
		machineRepo: &fakeMachineRepo{machines: map[string]*models.Machine{"m1": machine}},
		encryption:  &fakeEncryption{rawKey: "raw-key"},
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
	machine := &models.Machine{
		ID:              "m1",
		Hostname:        "nonexistent.invalid",
		Port:            22,
		SSHUsername:     "root",
		SSHKeyEncrypted: []byte("ignored"),
	}
	d := newSSHDialer(sshDialerConfig{
		machineRepo:    &fakeMachineRepo{machines: map[string]*models.Machine{"m1": machine}},
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
