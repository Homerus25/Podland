package sshexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/Homerus25/podland/backend/internal/rescache"
	"golang.org/x/crypto/ssh"
)

// dialer is the seam between the Service and the actual SSH work. Production
// wires *sshDialer (which manages a per-host sshClient cache); tests
// wire a fake to script Dial/Run behaviour.
type dialer interface {
	// Dial returns a sessionRunner for the given host, dialing (and
	// caching) a fresh sshClient on a cache miss.
	Dial(ctx context.Context, hostID string) (sessionRunner, error)
	// Invalidate drops the cached sshClient for the given host so the
	// next Dial re-dials. Called by the Service after a real session
	// failure (the cached client is likely stale). Fail-fast: the client
	// closes even if a call is mid-flight (short-lived execs; the affected
	// call surfaces the close as a session failure).
	Invalidate(hostID string)
	// RemoveHost invalidates a host's cached sshClient gracefully
	// (mirrors connections.Manager.RemoveHost): the entry leaves the
	// cache immediately, in-flight execs finish on the client they already
	// hold, and the client closes when the last of them releases. Used at
	// host-deletion time so no live SSH session outlives the host.
	RemoveHost(hostID string)
	// Close stops any background goroutines owned by the dialer (the
	// cleanup loop) and releases cached clients. It must be safe to call
	// multiple times. Used by Service.Close so internal/app.App.Close can
	// shut the whole backend down without leaking goroutines.
	Close() error
}

// sessionRunner runs a single command on a remote host. Each Dial returns
// a fresh runner; the underlying sshClient may be shared across runners
// (*ssh.Client is safe for concurrent NewSession calls).
type sessionRunner interface {
	// Run executes cmd on the remote host and returns its captured
	// stdout/stderr/exit-code. A non-zero exit code must NOT be reported
	// as an error — only real failures (session closed, network drop, ctx
	// cancellation surfacing as something other than an exit code) return
	// a non-nil error.
	Run(ctx context.Context, cmd string) (ExecResult, error)
	// RunStream is the streaming variant of Run: it invokes onLine for each
	// output line as it arrives (tagged with its stream) while still
	// capturing the full result. Same error semantics as Run.
	RunStream(ctx context.Context, cmd string, onLine func(OutputLine)) (ExecResult, error)
	// Close releases this exec's hold on the cached client (the reference
	// taken by Dial). For a host removed via RemoveHost, the last
	// release closes the client — the graceful teardown.
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

// sshDialer is the production dialer. It owns a per-host sshClient cache
// (internal/rescache, architecture review 2026-10-19 candidate C1): the
// cache owns the whole lifecycle — acquire with dial dedup (the dial runs
// outside the lock, so cold dials to different hosts never serialize),
// refcounted release, LRU-idle eviction that never closes an in-flight
// client, idle cleanup that skips in-use clients, graceful Remove, and
// fail-fast Invalidate. This adapter hands over its dial and close and keeps
// its own session-runner projection.
type sshDialer struct {
	hostRepo       hostRepo
	encryption     encryptionSvc
	connectTimeout time.Duration

	// cache owns the per-host sshClient lifecycle — internal/rescache.
	cache *rescache.Cache[sshClient]

	// dialSSH is the seam that actually produces an sshClient. In
	// production it is the real handshake; in dialer-level tests it is
	// swapped for a fake so the cache can be exercised without a live SSH
	// server.
	dialSSH func(ctx context.Context, hostID string) (sshClient, error)

	// dialCount counts how many times dialSSH has been invoked. Test-only
	// observable; production never reads it.
	dialCount atomic.Int32
}

// sshDialerConfig configures an sshDialer.
type sshDialerConfig struct {
	hostRepo        hostRepo
	encryption      encryptionSvc
	maxSize         int
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	connectTimeout  time.Duration
}

func newSSHDialer(cfg sshDialerConfig) *sshDialer {
	d := &sshDialer{
		hostRepo:       cfg.hostRepo,
		encryption:     cfg.encryption,
		connectTimeout: cfg.connectTimeout,
	}
	d.dialSSH = d.dialSSHProd
	d.cache = rescache.New(rescache.Options[sshClient]{
		Dial:            d.dialForCache,
		Close:           closeSSHClient,
		MaxSize:         cfg.maxSize,
		IdleTimeout:     cfg.idleTimeout,
		CleanupInterval: cfg.cleanupInterval,
	})
	return d
}

// dialForCache is the rescache Dial seam: it runs the dialSSH seam and counts
// the successful dial (test-only observable). The dial runs outside the
// cache lock, so cold dials to different hosts never serialize and
// concurrent cold dials for one host share one dial.
func (d *sshDialer) dialForCache(ctx context.Context, hostID string) (sshClient, error) {
	client, err := d.dialSSH(ctx, hostID)
	if err != nil {
		return nil, err
	}
	d.dialCount.Add(1)
	return client, nil
}

// closeSSHClient tears a cached sshClient down.
func closeSSHClient(c sshClient) {
	_ = c.Close()
}

// Dial returns a sessionRunner for hostID. On a cache miss it dials a new
// sshClient (and may evict the LRU idle entry first if the cache is full).
// On a cache hit the existing client is reused. Each Dial takes one
// reference on the entry, released by the returned runner's Close — that is
// what lets RemoveHost wait out in-flight execs.
func (d *sshDialer) Dial(ctx context.Context, hostID string) (sessionRunner, error) {
	client, release, err := d.cache.Acquire(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return &sshSessionRunner{client: client, release: release}, nil
}

// Invalidate closes and drops the cached sshClient for hostID. Safe to
// call when no entry exists (no-op). Fail-fast: the client closes even if a
// call is mid-flight (short-lived execs; the affected call surfaces the close
// as a session failure).
func (d *sshDialer) Invalidate(hostID string) {
	d.cache.Invalidate(hostID)
}

// RemoveHost invalidates a host's cached sshClient gracefully: the
// entry leaves the cache immediately (a later exec simply dials again), and
// the client closes as soon as the last in-flight exec releases — an exec
// already running on the host finishes untouched. Unknown hosts are a
// no-op. Mirrors connections.Manager.RemoveHost.
func (d *sshDialer) RemoveHost(hostID string) {
	d.cache.Remove(hostID)
}

// Close stops the cleanup goroutine (blocking until it has exited) and
// drops all cached clients. Safe to call any number of times.
//
// Mirrors connections.ConnectionManager.Stop so internal/app.App.Close has
// symmetric "no goroutine left running" guarantees across both background
// loops it owns.
func (d *sshDialer) Close() error {
	d.cache.Stop()
	d.cache.InvalidateAll()
	return nil
}

// dialSSHProd is the production dial path: fetch the host, decrypt its
// SSH key, parse the key, dial TCP (honouring ctx), perform the SSH
// handshake, and return the resulting *ssh.Client. Any failure is returned
// unwrapped-by-retry (docs/architecture-decisions.md §4 fail-fast).
func (d *sshDialer) dialSSHProd(ctx context.Context, hostID string) (sshClient, error) {
	host, err := d.hostRepo.GetByID(hostID)
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to fetch host %q: %w", hostID, err)
	}
	if host == nil {
		return nil, fmt.Errorf("sshexec: host not found: %s", hostID)
	}

	privateKey, err := d.encryption.Decrypt(host.SSHKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to decrypt SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("sshexec: failed to parse SSH key: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", host.Hostname, host.Port)
	// SECURITY: InsecureIgnoreHostKey matches the existing connection
	// factory's posture (it does not verify known_hosts either). This is a
	// known gap; host-key verification should be added alongside
	// host-registration capturing the host public key.
	cfg := &ssh.ClientConfig{
		User:            host.SSHUsername,
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

// sshSessionRunner is the production sessionRunner. It wraps one cached
// sshClient on behalf of a single exec; each Run opens a fresh *ssh.Session.
type sshSessionRunner struct {
	client  sshClient
	release rescache.ReleaseFunc
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

// RunStream is the streaming variant of Run: it invokes onLine for each
// output line as it arrives (tagged with its stream) while still capturing
// the full result. Same error semantics as Run.
func (r *sshSessionRunner) RunStream(ctx context.Context, cmd string, onLine func(OutputLine)) (ExecResult, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return ExecResult{}, fmt.Errorf("sshexec: failed to open session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	lineGroup := newLineTeeGroup(onLine)
	stdoutTee := newLineTeeWriter(StdoutStream, lineGroup, &stdout)
	stderrTee := newLineTeeWriter(StderrStream, lineGroup, &stderr)
	session.Stdout = stdoutTee
	session.Stderr = stderrTee

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

	// Emit any trailing partial lines so the callback sees the complete
	// output before the result is returned.
	stdoutTee.Flush()
	stderrTee.Flush()

	return classifyRunResult(runErr, ctx.Err(), stdout.String(), stderr.String())
}

func (r *sshSessionRunner) Close() error {
	// Release this exec's hold on the cached client. A removed host's
	// entry closes its client here when the last in-flight exec lets go —
	// that is RemoveHost's graceful teardown.
	r.release()
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
