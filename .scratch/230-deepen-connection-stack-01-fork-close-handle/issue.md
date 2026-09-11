---
title: Fork — additive NewConnectionWithHandle returning a close handle
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - podman-fork
  - refactor
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

In the podman fork (`~/repos/podman`), **on a new git branch**, add an additive API next to the existing dial entry point in `pkg/bindings/connection.go`:

```go
// NewConnectionWithHandle behaves exactly like NewConnectionWithOptions
// but additionally returns a CloseFunc that tears down the underlying
// SSH session. Calling it more than once is a no-op.
func NewConnectionWithHandle(ctx context.Context, opts Options) (context.Context, CloseFunc, error)
```

(This signature is a design-session decision; keep it exactly.)

- **Additive, not breaking**: `NewConnectionWithOptions` keeps its current signature and delegates to the new path, discarding the handle. No other fork caller churns; the diff stays tiny and upstream-merge-friendly.
- The handle must close the SSH session that today is captured unreachable inside the transport's `dialContext` closure (`ssh.Dial` result in `NewConnectionWithOptions`) — that closure is the only thing that can reach it. Closing may also close idle transport connections; it must not affect other machines' connections.
- Develop on the new branch as requested, then **merge into the fork's mainline working branch**: Podland's `backend/go.mod` pins the fork by local path (`replace go.podman.io/podman/v6 => ...`), so Podland builds against whatever is checked out — a lone branch would strand the build. Podland's `go.mod` is not touched by this ticket.

## Acceptance criteria

- [ ] New branch in `~/repos/podman` carries the change; merged into the fork's mainline branch afterwards
- [ ] `NewConnectionWithHandle` returns a context whose transport behaves identically to `NewConnectionWithOptions`'s, plus a working `CloseFunc`
- [ ] `CloseFunc` closes the SSH session for that connection only; idempotent; safe to call while requests are in flight
- [ ] `NewConnectionWithOptions` signature unchanged; existing fork callers untouched; it delegates to the new path
- [ ] `go build ./...` in `backend/` still succeeds against the merged fork
- [ ] No Podland behavior changes yet — nothing in Podland calls the new API

## Blocked by

None (can start immediately).
