---
title: Deepen the connection stack — one ConnectionManager seam with real teardown
parent: none
date: 2026-09-07
status: ready-for-agent
labels:
  - connections
  - backend
  - refactor
author: vibe-agent
---

## Problem Statement

The connection stack (`internal/connections`: manager + cache + factory, ~690 lines across three modules) is shallow where it claims depth:

- **Advertised depth is dead or unenforced.** The "request queue" only fires after a failed dial and re-calls the same dial once — a de-facto retry that contradicts the documented fail-fast value. `maxConcurrent` only sizes a channel buffer. The cache's `GetOrCreate` / `MarkActive` / `MarkIdle` and the factory's `CreateConnection` have zero production callers, kept alive by tests. The manager stores `machineRepo`, `encryption`, and `activeConnections` without ever reading them.
- **The interface leaks refcount discipline.** 36 call sites across five domains must pair `GetConnection` with `defer Release`; forgetting `Release` pins refCount above zero, so LRU idle eviction and the 15-minute cleanup silently skip that machine forever.
- **The 502 `CONNECTION_FAILED` contract has drifted.** Every service maps the error and logs the cause itself; 14 of 49 sites silently drop the cause log the backend conventions require.
- **Eviction and cleanup tear nothing down.** The fork's `NewConnectionWithOptions` returns no handle — the SSH session is captured in a closure — so eviction, idle cleanup, and (soon) machine deletion would leak an authenticated SSH session to the host until backend restart.
- **The interface is homeless.** `ConnectionManagerInterface` lives in the machines domain; six domains import machines purely for infrastructure.

The design below was settled in an architecture grilling session on 2026-09-07; the inline type shapes are that session's decisions, not suggestions.

## Solution

Merge the three modules into one deep `ConnectionManager` behind a five-method `Manager` interface that lives in `internal/connections`:

- `WithConnection` owns the pairing (callback shape — release is invisible to callers), the 502 mapping, and the cause log. Call sites lose the whole prologue ritual.
- Per-machine in-flight dial dedup replaces the queue; the dial moves outside the global lock (today cold dials to different machines serialize behind one mutex).
- The podman fork gains an additive close handle, so LRU eviction, idle cleanup, and the new `RemoveMachine` genuinely close the SSH session. `RemoveMachine` is graceful: the entry leaves the map at once, the handle closes when the last in-flight callback returns.
- Migration is expand–contract: the deepened module keeps `GetConnection`/`Release` shims satisfying the old interface; volumes migrates first as the reference slice; then per-domain sweep; then the contract ticket deletes the old surface and rehomes the interface.

## Out of Scope

- **sshexec stays separate.** Its fetch+decrypt prologue duplicates two lines of five; two copies sit below the conventions' third-copy extraction threshold. Revisit if a third dialer appears.
- **Wiring `RemoveMachine` into `MachineService.Delete`** — that is `.scratch/020-machine-deletion-03-connection-cache`, which consumes this seam. This epic only delivers `RemoveMachine` on the interface.
- **Admission control / request queuing** — deliberately deleted, not implemented. Fail-fast is the documented value; dedup is the real missing depth.
- **API contract changes** — response envelopes are unchanged; the only doku change is removing the dead `CONNECTION_QUEUE_TIMEOUT_SECONDS` row.
- **Streaming orchestration consolidation** (logs endpoints through `streaming.Run`) — a separate candidate from the same review, untouched here.

## Slices

1. `230-deepen-connection-stack-01-fork-close-handle` — additive `NewConnectionWithHandle` on a new fork branch, merged to fork mainline — no blockers
2. `230-deepen-connection-stack-02-deep-connection-manager` — one deep module behind the `Manager` interface; queue + dead surface deleted; old shims kept (expand) — blocked by 230-01
3. `230-deepen-connection-stack-03-volumes-reference-migration` — volumes crosses `WithConnection`; reference pattern for the sweep — blocked by 230-02
4. `230-deepen-connection-stack-04-containers-images-migration` — containers + images sweep — blocked by 230-03
5. `230-deepen-connection-stack-05-quadlets-secrets-migration` — quadlets + secrets sweep (the cause-log drift domains) — blocked by 230-03
6. `230-deepen-connection-stack-06-machines-and-contract` — machines migrates; old interface, shims, and stale docs deleted (contract) — blocked by 230-04 and 230-05
