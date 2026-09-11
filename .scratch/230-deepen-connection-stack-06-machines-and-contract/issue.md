---
title: Machines migrates; old interface and shims deleted (contract)
parent: Deepen the connection stack — one ConnectionManager seam with real teardown
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - machines
  - backend
  - refactor
  - docs
author: vibe-agent
---

## Parent

Deepen the connection stack (see `.scratch/230-deepen-connection-stack/issue.md`).

## What to build

The contract phase of the expand–contract migration: machines is the last domain on the old interface, and once it crosses, the old surface is deleted everywhere.

- **Machines migrates**: `MachineService` depends on `connections.Manager`. Its usage is mostly `ProbeMachine` / `TestConnection` (already on the new interface, timeout semantics unchanged); any direct `GetConnection`/`Release` use in the machines service migrates to `WithConnection` with the volumes closure shape. The registration flow (`Create` → `TestConnection`) and the status endpoint (`GetMachineStatus` → `ProbeMachine`) must behave identically.
- **Delete the old surface**:
  - `ConnectionManagerInterface` (and only that) from `internal/domain/machines/service_interfaces.go` — `EncryptionServiceInterface` stays.
  - The `GetConnection`/`Release` shims on the concrete `ConnectionManager`.
  - The old methods on `MockConnectionManager` in `testutils`; the mock trims to the `Manager` interface.
- **Rehome the seam**: every domain imports `connections` for the seam; no domain imports `machines` purely for infrastructure anymore.
- **Docs tell the new truth**:
  - `docs/backend-conventions.md`: the 502 `CONNECTION_FAILED` cause log now lives at the connection seam (`WithConnection`), not at each service site; update the reference-implementation pointers for the new prologue shape.
  - `CONTEXT.md` §4 (Concurrency & Performance): fail-fast on connection failure, per-machine in-flight dial dedup, one cached connection per machine with real teardown on evict/idle/removal — the semaphore/queuing promises are gone because the queue never existed.
- **Full gates**: `go test ./...`, plus `gofmt -w test/integration` if any build-tagged file was touched (per conventions).

Note: wiring `RemoveMachine` into `MachineService.Delete` is **not** this ticket — that is `.scratch/020-machine-deletion-03-connection-cache`, which now consumes a seam that actually tears down.

## Acceptance criteria

- [x] `MachineService` compiles against `connections.Manager`; registration and status behavior unchanged
- [x] No references to `ConnectionManagerInterface`, `GetConnection`, or `Release` remain anywhere in `backend/` (grep clean, tests included)
- [x] `MockConnectionManager` implements exactly the five `Manager` methods
- [x] No domain imports `machines` solely for the connection seam
- [x] `docs/backend-conventions.md` and `CONTEXT.md` §4 updated; no stale queue/semaphore claims anywhere in the docs
- [x] `go test ./...` green; gofmt clean including build-tagged integration files

## Blocked by

- `230-deepen-connection-stack-04-containers-images-migration`
- `230-deepen-connection-stack-05-quadlets-secrets-migration`
