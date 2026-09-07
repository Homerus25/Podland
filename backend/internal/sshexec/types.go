package sshexec

import "context"

// ExecOptions carries optional parameters for an SSH exec call. It is kept
// narrow on purpose so the SSHExecInterface stays easy to mock; future
// capabilities (env, stdin, working dir) can be added without breaking
// existing callers.
type ExecOptions struct {
	// Args are the arguments to pass to the command. They are shell-quoted
	// and appended to the command string before execution, so callers pass
	// already-split values (e.g. Args: []string{"start", "web.service"}).
	Args []string
}

// ExecResult holds the captured output of a remote command run. A non-zero
// ExitCode is NOT modelled as an error: the caller decides how to interpret
// it (e.g. systemctl exit codes mapping to specific AppErrors).
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SSHExecInterface defines the ability to run a command on a remote machine
// over SSH using that machine's stored credentials.
//
// This interface is intentionally SEPARATE from the podman connection seam
// (internal/connections): the domain services depend on connections.Manager
// for podman calls and add sshexec.SSHExecInterface only where shell access
// is needed. The first consumer is the quadlet lifecycle service (070-06),
// which runs `systemctl` over SSH.
//
// (It lives here rather than in a domain package because the machines tests
// already import internal/testutils and MockSSHExec needs to reference these
// types; a domain home would create a testutils→domain→testutils import
// cycle.)
type SSHExecInterface interface {
	Exec(ctx context.Context, machineID, userID, command string, opts ExecOptions) (ExecResult, error)
	// ExecStream is the streaming variant of Exec: it invokes onLine for
	// each output line as it arrives, tagged with the stream it came from
	// (OutputLine.Stream), while still returning the fully captured result
	// so callers can interpret the exit code as with Exec.
	ExecStream(ctx context.Context, machineID, userID, command string, opts ExecOptions, onLine func(OutputLine)) (ExecResult, error)
}
