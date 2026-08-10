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
// This interface is intentionally SEPARATE from machines.ConnectionManagerInterface:
// the existing images/containers/quadlets services depend only on
// ConnectionManagerInterface and are completely unaffected by this
// capability. The first consumer is the quadlet lifecycle service (070-06),
// which runs `systemctl` over SSH.
//
// (The issue suggested internal/domain/machines/service_interfaces.go as the
// home — "e.g." — but co-locating it with ConnectionManagerInterface would
// create an import cycle, because the existing machines tests already
// import internal/testutils and MockSSHExec needs to reference these types.
// See the long comment in internal/domain/machines/service_interfaces.go.)
type SSHExecInterface interface {
	Exec(ctx context.Context, machineID, userID, command string, opts ExecOptions) (ExecResult, error)
}
