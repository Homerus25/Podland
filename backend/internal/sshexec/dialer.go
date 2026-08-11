package sshexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// dialer is the seam between the Service and the actual SSH work. Production
// wires *sshDialer (which manages a per-machine sshClient cache); tests
// wire a fake to script Dial/Run behaviour.
type dialer interface {
	// Dial returns a sessionRunner for the given machine, dialing (and
	// caching) a fresh sshClient on a cache miss.
	Dial(ctx context.Context, machineID string) (sessionRunner, error)
	// Invalidate drops the cached sshClient for the given machine so the
	// next Dial re-dials. Called by the Service after a real session
	// failure (the cached client is likely stale).
	Invalidate(machineID string)
	// Close stops any background goroutines owned by the dialer (the
	// cleanup loop) and releases cached clients. It must be safe to call
	// multiple times. Used by Service.Close so internal/app.App.Close can
	// shut the whole backend down without leaking goroutines.
	Close() error
}

// sessionRunner runs a single command on a remote machine. Each Dial returns
// a fresh runner; the underlying sshClient may be shared across runners
// (*ssh.Client is safe for concurrent NewSession calls).
type sessionRunner interface {
	// Run executes cmd on the remote machine and returns its captured
	// stdout/stderr/exit-code. A non-zero exit code must NOT be reported
	// as an error — only real failures (session closed, network drop, ctx
	// cancellation surfacing as something other than an exit code) return
	// a non-nil error.
	Run(ctx context.Context, cmd string) (ExecResult, error)
	// Close releases any per-call resources (the underlying sshClient is
	// managed by the dialer cache and is NOT closed here).
	Close() error
}

// sshClient is the narrow subset of *ssh.Client that the dialer/cache cares
// about. Defining it as an interface lets dialer-level tests substitute a
// fake client that records Close() calls — *ssh.Client is a concrete struct
// and otherwise cannot be mocked.
//
// *ssh.Client satisfies this interface implicitly.
type sshClient interface {
	Close() error
	NewSession() (*ssh.Session, error)
}

// sshDialer is the production dialer. It owns a per-machine sshClient cache
// with LRU eviction and a background cleanup loop, mirroring
// internal/connections/cache.go.
type sshDialer struct {
	machineRepo machineRepo
	encryption  encryptionSvc

	maxSize         int
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	connectTimeout  time.Duration

	// dialSSH is the seam that actually produces an sshClient. In
	// production it is the real handshake; in dialer-level tests it is
	// swapped for a fake so the cache can be exercised without a live SSH
	// server.
	dialSSH func(ctx context.Context, machineID string) (sshClient, error)

	mu      sync.Mutex
	clients map[string]*sshClientEntry
	stopCh  chan struct{}
	// stoppedCh is closed by cleanupLoop on its way out so Close can block
	// until the goroutine has truly exited. nil when no cleanup goroutine
	// was started (cleanupInterval == 0); in that case Close doesn't wait.
	// The pointer is set once in newSSHDialer before the goroutine starts
	// and never modified after, so reading it in Close is race-free.
	stoppedCh chan struct{}
	// stopOnce makes Close idempotent so repeat callers don't observe a
	// panic on a double-close of stopCh.
	stopOnce sync.Once

	// dialCount counts how many times dialSSH has been invoked. Test-only
	// observable; production never reads it. Lives behind d.mu so the
	// fake dialSSH can safely increment it.
	dialCount int
}

type sshClientEntry struct {
	client   sshClient
	lastUsed time.Time
}

// sshDialerConfig configures an sshDialer.
type sshDialerConfig struct {
	machineRepo     machineRepo
	encryption      encryptionSvc
	maxSize         int
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	connectTimeout  time.Duration
}

func newSSHDialer(cfg sshDialerConfig) *sshDialer {
	d := &sshDialer{
		machineRepo:     cfg.machineRepo,
		encryption:      cfg.encryption,
		maxSize:         cfg.maxSize,
		idleTimeout:     cfg.idleTimeout,
		cleanupInterval: cfg.cleanupInterval,
		connectTimeout:  cfg.connectTimeout,
		clients:         make(map[string]*sshClientEntry),
		stopCh:          make(chan struct{}),
	}
	d.dialSSH = d.dialSSHProd
	if cfg.cleanupInterval > 0 {
		d.stoppedCh = make(chan struct{})
		go d.cleanupLoop()
	}
	return d
}

// Dial returns a sessionRunner for machineID. On a cache miss it dials a new
// sshClient (and may evict the LRU entry first if the cache is full).
// On a cache hit the existing client is reused and lastUsed is bumped.
func (d *sshDialer) Dial(ctx context.Context, machineID string) (sessionRunner, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.clients[machineID]; ok {
		entry.lastUsed = time.Now()
		return &sshSessionRunner{client: entry.client}, nil
	}

	if len(d.clients) >= d.maxSize {
		d.evictLRU()
	}

	client, err := d.dialSSH(ctx, machineID)
	if err != nil {
		return nil, err
	}
	d.dialCount++
	d.clients[machineID] = &sshClientEntry{client: client, lastUsed: time.Now()}
	return &sshSessionRunner{client: client}, nil
}

// Invalidate closes and drops the cached sshClient for machineID. Safe to
// call when no entry exists (no-op).
func (d *sshDialer) Invalidate(machineID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.clients[machineID]; ok {
		_ = entry.client.Close()
		delete(d.clients, machineID)
	}
}

// Close stops the cleanup goroutine (blocking until it has exited) and
// drops all cached clients. Safe to call any number of times — sync.Once
// guards the stopCh close so repeat callers don't observe a panic, and
// waiting on an already-closed stoppedCh returns immediately.
//
// If no cleanup goroutine was started (cleanupInterval == 0) Close returns
// without blocking — there's nothing to join.
//
// Mirrors connections.ConnectionManager.Stop so internal/app.App.Close has
// symmetric "no goroutine left running" guarantees across both background
// loops it owns.
func (d *sshDialer) Close() error {
	d.stopOnce.Do(func() {
		close(d.stopCh)
	})
	if d.stoppedCh != nil {
		<-d.stoppedCh
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, entry := range d.clients {
		_ = entry.client.Close()
		delete(d.clients, id)
	}
	return nil
}

// evictLRU drops the entry with the oldest lastUsed. Must be called with
// d.mu held.
//
// Unlike connections.ConnectionCache (which tracks refCount/isActive and only
// evicts idle entries), each SSH exec is short-lived and *ssh.Client is safe
// for concurrent NewSession calls — so we simply pick the oldest entry.
// Under sustained churn this could in theory close a client mid-Run; the
// affected call surfaces that as a real session failure (which the Service
// maps to an error and invalidates the now-stale entry).
func (d *sshDialer) evictLRU() {
	var lruID string
	var lruTime time.Time
	for id, entry := range d.clients {
		if lruID == "" || entry.lastUsed.Before(lruTime) {
			lruID = id
			lruTime = entry.lastUsed
		}
	}
	if lruID != "" {
		if entry := d.clients[lruID]; entry != nil {
			_ = entry.client.Close()
		}
		delete(d.clients, lruID)
	}
}

// cleanupLoop periodically drops idle clients older than IdleTimeout. It
// closes stoppedCh on exit so Close can block until the goroutine has
// truly terminated (mirroring connections.ConnectionManager.processQueue).
func (d *sshDialer) cleanupLoop() {
	defer close(d.stoppedCh)
	ticker := time.NewTicker(d.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.cleanup()
		case <-d.stopCh:
			return
		}
	}
}

// cleanup closes and drops entries whose lastUsed is older than IdleTimeout.
func (d *sshDialer) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for id, entry := range d.clients {
		if now.Sub(entry.lastUsed) > d.idleTimeout {
			_ = entry.client.Close()
			delete(d.clients, id)
		}
	}
}

// dialSSHProd is the production dial path: fetch the machine, decrypt its
// SSH key, parse the key, dial TCP (honouring ctx), perform the SSH
// handshake, and return the resulting *ssh.Client. Any failure is returned
// unwrapped-by-retry (CONTEXT.md §4 fail-fast).
func (d *sshDialer) dialSSHProd(ctx context.Context, machineID string) (sshClient, error) {
	machine, err := d.machineRepo.GetByID(machineID)
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to fetch machine %q: %w", machineID, err)
	}
	if machine == nil {
		return nil, fmt.Errorf("sshexec: machine not found: %s", machineID)
	}

	privateKey, err := d.encryption.Decrypt(machine.SSHKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to decrypt SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to parse SSH key: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", machine.Hostname, machine.Port)
	// SECURITY: InsecureIgnoreHostKey matches the existing connection
	// factory's posture (it does not verify known_hosts either). This is a
	// known gap; host-key verification should be added alongside
	// machine-registration capturing the host public key.
	cfg := &ssh.ClientConfig{
		User:            machine.SSHUsername,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         d.connectTimeout,
	}

	netDialer := &net.Dialer{Timeout: d.connectTimeout}
	conn, err := netDialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sshexec: SSH handshake to %s failed: %w", addr, err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

// sshSessionRunner is the production sessionRunner. It wraps a single
// sshClient; each Run opens a fresh *ssh.Session.
type sshSessionRunner struct {
	client sshClient
}

func (r *sshSessionRunner) Run(ctx context.Context, cmd string) (ExecResult, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return ExecResult{}, fmt.Errorf("sshexec: failed to open session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Honour context cancellation by signalling the remote process. The
	// goroutine exits when Run returns (done) or when ctx is cancelled.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Best-effort kill. Errors are ignored — ctx.Err() is
			// returned below regardless of whether the signal landed.
			_ = session.Signal(ssh.SIGKILL)
		case <-done:
		}
	}()

	runErr := session.Run(cmd)
	close(done)

	return classifyRunResult(runErr, ctx.Err(), stdout.String(), stderr.String())
}

func (r *sshSessionRunner) Close() error {
	// The sshClient is owned by the dialer cache; nothing to free here.
	return nil
}

// classifyRunResult maps a session.Run outcome plus any context error into
// the SSH layer's contract:
//
//   - context cancellation takes precedence (the Service contract requires
//     Exec to return ctx.Err() when the caller abandoned the call, even if
//     the command also produced an exit code or stdout),
//   - a nil runErr is a clean exit 0,
//   - an *ssh.ExitError is a non-zero exit (NOT propagated as an error —
//     callers interpret res.ExitCode themselves),
//   - anything else is a real failure (network drop, protocol error, ...).
//
// Extracted as a pure function so the precedence rules are unit-testable
// without a live SSH server.
func classifyRunResult(runErr error, ctxErr error, stdout, stderr string) (ExecResult, error) {
	if ctxErr != nil {
		return ExecResult{}, ctxErr
	}
	var exitErr *ssh.ExitError
	switch {
	case runErr == nil:
		return ExecResult{Stdout: stdout, Stderr: stderr, ExitCode: 0}, nil
	case errors.As(runErr, &exitErr):
		// Non-zero exit — not an error from the SSH layer's POV.
		return ExecResult{Stdout: stdout, Stderr: stderr, ExitCode: exitErr.ExitStatus()}, nil
	default:
		return ExecResult{}, fmt.Errorf("sshexec: command failed: %w", runErr)
	}
}
