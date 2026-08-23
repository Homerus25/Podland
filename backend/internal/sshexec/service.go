// Package sshexec runs commands on remote machines over SSH using each
// machine's stored credentials (host, user, encrypted private key — all
// already persisted at machine-registration time).
//
// The package defines SSHExecInterface and a production *Service
// implementation. The underlying *ssh.Client is cached per machine
// (mirroring the connection-cache discipline and LRU eviction in
// internal/connections), and SSH failures fail fast (no retry, per
// CONTEXT.md §4).
package sshexec

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Homerus25/podland/backend/internal/encryption"
	"github.com/Homerus25/podland/backend/internal/models"
)

// Compile-time check that *Service satisfies SSHExecInterface.
var _ SSHExecInterface = (*Service)(nil)

// machineRepo is the narrow subset of *models.MachineRepository that the
// SSH-exec dialer depends on. Defining it locally keeps the constructor
// signature honest about what we actually need and lets tests supply a fake
// without dragging in the SQL-backed concrete type.
//
// *models.MachineRepository satisfies this implicitly.
type machineRepo interface {
	GetByID(id string) (*models.Machine, error)
}

// encryptionSvc is the narrow subset of *encryption.EncryptionService that
// the SSH-exec dialer depends on. *encryption.EncryptionService satisfies
// this implicitly.
type encryptionSvc interface {
	Decrypt(data []byte) (string, error)
}

// Service runs commands on remote machines over SSH and caches the
// underlying *ssh.Client per machine.
//
// The actual SSH work is delegated to a dialer — a small internal seam that
// keeps Exec testable without a live SSH server (the dialer and runner are
// interfaces; production wires sshDialer, tests wire a fake).
type Service struct {
	dialer dialer
}

// Options configures a Service. Mirrors the Options-struct idiom used by
// connections.ConnectionFactoryOptions.
type Options struct {
	MachineRepo *models.MachineRepository
	Encryption  *encryption.EncryptionService

	// MaxClients caps the per-machine *ssh.Client cache (LRU-evicted when
	// full). Defaults to 100 when zero, matching the connection-cache
	// default documented in CONTEXT.md §4.
	MaxClients int

	// IdleTimeout controls how long a cached *ssh.Client may sit unused
	// before the background cleanup loop closes and drops it. Defaults to
	// 15 minutes when zero, matching the connection-cache idle timeout.
	IdleTimeout time.Duration

	// CleanupInterval is the period of the background cleanup loop.
	// Defaults to 30 seconds when zero, matching the connection cache.
	CleanupInterval time.Duration

	// ConnectTimeout is applied to the TCP dial + SSH handshake. Defaults
	// to 30 seconds when zero, matching the connection factory posture.
	ConnectTimeout time.Duration
}

// NewService constructs a production Service backed by a real sshDialer.
// The returned Service is safe for concurrent use.
func NewService(opts Options) *Service {
	maxClients := opts.MaxClients
	if maxClients <= 0 {
		maxClients = 100
	}
	idleTimeout := opts.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 15 * time.Minute
	}
	cleanupInterval := opts.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = 30 * time.Second
	}
	connectTimeout := opts.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 30 * time.Second
	}

	d := newSSHDialer(sshDialerConfig{
		machineRepo:     opts.MachineRepo,
		encryption:      opts.Encryption,
		maxSize:         maxClients,
		idleTimeout:     idleTimeout,
		cleanupInterval: cleanupInterval,
		connectTimeout:  connectTimeout,
	})
	return &Service{dialer: d}
}

// newServiceWithDialer is the test seam: it constructs a Service whose SSH
// work is delegated to the supplied dialer. Production callers should use
// NewService.
func newServiceWithDialer(d dialer) *Service {
	return &Service{dialer: d}
}

// Close stops background goroutines owned by the dialer (the cleanup loop)
// and releases cached SSH clients. Safe to call any number of times.
//
// Used by internal/app.App.Close so the in-process test harness and the
// production server share the same shutdown path.
func (s *Service) Close() error {
	return s.dialer.Close()
}

// Exec implements SSHExecInterface.
//
// The command string and opts.Args are concatenated into a single
// shell-quoted command string (golang.org/x/crypto/ssh's Session.Run only
// accepts a single string). The runner captures stdout, stderr, and the
// exit code. A non-zero exit code is NOT an error — callers interpret exit
// codes themselves (e.g. systemctl's unit-not-loaded code).
//
// Fail-fast: on a real SSH/session failure (anything other than a non-zero
// exit code), the cached *ssh.Client for this machine is invalidated so the
// next call re-dials, and the error is returned unwrapped-by-retry.
func (s *Service) Exec(ctx context.Context, machineID, userID, command string, opts ExecOptions) (ExecResult, error) {
	_ = userID // accepted for interface symmetry with ConnectionManagerInterface;
	// permission checks happen at the handler layer, matching the
	// connection manager's posture (it also ignores userID beyond metadata).

	cmd := shellQuote(command, opts.Args)

	runner, err := s.dialer.Dial(ctx, machineID)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sshexec: failed to dial machine %q: %w", machineID, err)
	}
	defer runner.Close()

	result, runErr := runner.Run(ctx, cmd)
	if runErr != nil {
		// A real failure (network drop, session closed, ...). The cached
		// client is likely stale — drop it so the next call re-dials.
		s.dialer.Invalidate(machineID)
		return ExecResult{}, runErr
	}
	return result, nil
}

// shellQuote joins a command and its args into a single shell command string.
// The command itself is left unquoted (the caller controls it, just like the
// first argument to os/exec.Command); each arg is single-quoted so
// metacharacters in arg values cannot break out or be re-interpreted.
//
// Example: shellQuote("systemctl", []string{"start", "web.service"})
//
//	-> `systemctl 'start' 'web.service'`
func shellQuote(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	var b strings.Builder
	b.WriteString(command)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteByte('\'')
		b.WriteString(strings.ReplaceAll(a, "'", `'\''`))
		b.WriteByte('\'')
	}
	return b.String()
}

// (the real sshDialer / sessionRunner types live in dialer.go; the dialer
// interface is declared there to keep this file focused on the Service.)
