---
title: Deep ConnectionManager behind the Manager interface (expand)
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - backend
  - refactor
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

Rewrite `internal/connections` as **one module** (manager + cache + factory merged) behind this interface — the exact shape settled in the design session, defined in the `connections` package itself:

```go
// connections.Manager is the seam every machine-scoped podman call crosses.
type Manager interface {
	// WithConnection hands fn a live podman connection context and
	// releases it when fn returns. Dial failures are logged here
	// (op, machineID, userID, cause) and returned as
	// appErrors.ErrConnectionFailed — callers never map it themselves.
	WithConnection(ctx context.Context, userID, machineID, op string, fn func(connCtx context.Context) error) error
	ProbeMachine(machineID, userID string) error
	TestConnection(ctx context.Context, hostname string, port int, sshUsername, sshPrivateKey, podmanSocketPath string) error
	// RemoveMachine invalidates a machine's cached connection; the SSH
	// session closes once in-flight calls finish.
	RemoveMachine(machineID string)
	Stop()
}
```

No generic sugar on top (interface methods cannot be generic; a `WithResult[T]` helper was explicitly deferred).

**Implementation decisions (all settled):**

- The module owns lookup → decrypt → dial → per-machine LRU. The constructor takes what the factory needs today (machine repo, encryption, size limit); the manager's dead parameters die.
- **Deleted outright**: the queue goroutine (`processQueue`, `Run`, `connectionQueue`), the manager's never-read fields (`machineRepo`, `encryption`, `activeConnections`, its mutex), the cache's production-dead surface (`GetOrCreate`, `MarkActive`, `MarkIdle`), and the factory's dead `CreateConnection`. The queue was a de-facto single retry of a failed dial — fail-fast is the documented value.
- **Dial dedup replaces queuing**: concurrent cold acquisitions for the same machine share one dial (per-machine in-flight tracking), and the dial runs **outside** the global lock so cold dials to different machines stop serializing.
- **`WithConnection` owns the 502 contract**: on acquisition failure it warn-logs op + machineID + userID + cause and returns `appErrors.ErrConnectionFailed` (the module imports `internal/errors` — a leaf package, no cycle). Success path: run `fn`, release on return, propagate `fn`'s error untouched.
- **Real teardown**: every entry stores the `CloseFunc` from the fork's `NewConnectionWithHandle` (ticket 230-01). LRU eviction, the idle cleanup loop, and `RemoveMachine` all call it. The old "cleanup handled automatically by Go's HTTP client" comment was false — the SSH session leaked.
- **`RemoveMachine` is graceful**: the entry leaves the map immediately (new acquisitions fail on the machine lookup), the handle closes when the last in-flight `fn` returns, and a dial already in flight for that machine completes and closes right away (dead flag on the entry).
- **Expand phase**: the concrete type keeps `GetConnection`/`Release` shims so it still satisfies the old `machines.ConnectionManagerInterface` — un-migrated domains compile and stay green. Shims keep today's semantics (raw errors; callers still do their own mapping until their slice).
- **Config**: `CONNECTION_QUEUE_TIMEOUT_SECONDS` dies with the queue — remove the field, the env parsing, and its row in `doku/description_backend.md`. `Stop()` becomes idempotent shutdown of the cleanup goroutine (`App.Close` already calls it and must keep working).
- **Tests through the public interface with a fake dialer** (the `sshexec` dialer test pattern — inject the dial, no live SSH): release-on-return without caller discipline, 502 + cause-log ownership, two concurrent `WithConnection` calls → exactly one dial, eviction closes the handle, idle cleanup closes the handle, graceful `RemoveMachine`, `Stop` idempotence, and `ProbeMachine`/`TestConnection` timeout semantics preserved (their existing tests carry over). Tests pinning the deleted dead surface go.

## Acceptance criteria

- [ ] `Manager` interface defined in `internal/connections`; the concrete `ConnectionManager` satisfies both it and the old `machines.ConnectionManagerInterface`
- [ ] Queue goroutine, production-dead methods, dead fields, and `CONNECTION_QUEUE_TIMEOUT_SECONDS` (config + env parsing + doku row) are gone
- [ ] `WithConnection` logs op/machineID/userID/cause on dial failure and returns `appErrors.ErrConnectionFailed`; success path releases without caller discipline
- [ ] Concurrent cold acquisitions to one machine produce exactly one dial; cold dials to different machines do not serialize on one lock
- [ ] LRU eviction, idle cleanup, and `RemoveMachine` close the SSH handle; `RemoveMachine` waits for in-flight callbacks; close is idempotent
- [ ] Fake-dialer interface test suite green; probe/registration timeout tests carried over and green; `go test ./...` green with all domains still on the shims

## Blocked by

- `230-deepen-connection-stack-01-fork-close-handle` (the teardown needs the fork's `CloseFunc`)
