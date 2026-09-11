---
title: SSH-exec infrastructure for the connection layer
parent: 070-quadlets-management
date: 2026-08-02
status: ready-for-agent
labels:
  - quadlets
  - backend
  - feature
  - infrastructure
  - ssh
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## What to build

Add the ability to run a command on a remote machine over SSH from the backend. The current connection layer (`backend/internal/connections/`) is a **podman-socket tunnel only** — it dials SSH solely to forward the remote podman unix socket and exposes no exec/session capability. This slice adds a general-purpose SSH-exec capability that the quadlet lifecycle endpoints (070-06, chosen mechanism: `systemctl` over SSH) depend on.

This is a cross-cutting, general-purpose capability (any future feature needing remote commands can use it), published under this epic because the quadlet lifecycle work is its first consumer.

Design (keep it testable):

- Define a **new** `SSHExecInterface` (e.g. in `internal/domain/machines/service_interfaces.go`) with a method such as `Exec(ctx, machineID, userID, command string, opts ExecOptions) (ExecResult, error)` returning stdout/stderr/exit-code. Define it as a **separate** interface from `ConnectionManagerInterface` so the existing images/containers services (which depend on `ConnectionManagerInterface`) are unaffected.
- Implement the exec capability on `ConnectionManager` (or a new `SSHExecService` backed by the machine's stored SSH credentials — host, user, encrypted private key already in SQLite). Dial a `golang.org/x/crypto/ssh` client, open a `Session`, run the command, return the result. Cache/reuse the `*ssh.Client` per machine (mirroring the existing connection-cache discipline and LRU eviction), `fail fast` on errors (no retry, per CONTEXT.md §4).
- Expose it for dependency injection so the quadlet lifecycle service can depend on `SSHExecInterface` and be unit-tested with a mock (add a `MockSSHExec` to `internal/testutils/mock_services.go`).

No new HTTP endpoint is added by this slice — it is infrastructure consumed by 070-06.

## Acceptance criteria

- [ ] `SSHExecInterface` defined separately from `ConnectionManagerInterface` (no ripple to images/containers services)
- [ ] `ConnectionManager` (or a new `SSHExecService`) implements SSH exec against a remote machine using its stored SSH credentials
- [ ] The `*ssh.Client` is cached/reused per machine, consistent with the existing connection-cache and LRU-eviction behaviour
- [ ] Exec results include stdout, stderr, and exit code
- [ ] Fail-fast on SSH errors (no retry), per CONTEXT.md §4
- [ ] A `MockSSHExec` exists in `internal/testutils/mock_services.go`
- [ ] `go build ./...` succeeds, `go vet ./...` is clean, existing tests pass
- [ ] Unit tests cover success, non-zero exit code, SSH connection failure, and command cancellation via context

## Blocked by

None - can start immediately

## Technical notes

- The podman connection's SSH dial lives in `~/repos/podman/pkg/bindings/connection.go:178` (`sshClient`) but it tunnels to the unix socket and does not hand back an exec-capable client — Podland must dial its own `*ssh.Client` using the machine's stored credentials
- Machine SSH credentials (host, user, encrypted private key) are already persisted at machine-registration time and decrypted via `EncryptionServiceInterface`
- `golang.org/x/crypto/ssh` is already a dependency (used transitively by the podman connection)
- Keep the exec interface narrow so it can be mocked cleanly; do not broaden `ConnectionManagerInterface`
