--- 
title: Deletions and the Conventions Ruling
parent: route every streaming endpoint through one orchestration
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - cleanup
  - documentation
author: vibe-agent
---

## Parent

Part of the "route every streaming endpoint through one orchestration" epic (`250-streaming-orchestration/250-master-streaming-orchestration.md`).

## What to build

Delete the dead code the architecture review flagged beside the streaming convergence, and write the ruling down:

- **`ErrorHandler` dies.** The pass-through type in the errors module is replaced by a package-level `ToAppError(err)` that returns the wrapped-or-plain `*AppError`, or the internal-error sentinel for unknown errors. The auth and users handlers and the app wiring switch to it; their behaviour is unchanged.
- **The `events` domain dies.** The SSE stub (501 handler with `any`-typed dependencies, its service, and the `GET /api/machines/{id}/events` route) is deleted outright — it has never existed as a real endpoint. The machines API doku loses its event-endpoints section. A future SSE feature would start from the streaming module anyway.
- **The conventions ruling is written down.** `docs/backend-conventions.md`'s streaming section no longer says endpoints may "keep their own shape": every NDJSON endpoint enters via `streaming.Begin` and runs through `streaming.Run`; terminal error events for log streams are `"CODE: message"` log events; the exactly-one-terminal invariant (including `STREAM_ENDED` on clean end) is the module's, pinned in its own tests once.

## Acceptance criteria

- [ ] `errors.ToAppError(err)` exists; the `ErrorHandler` type and constructor are gone and nothing references them
- [ ] Auth and users handler behaviour is unchanged (their tests pass untouched apart from the mechanical call-site swap)
- [ ] The `events` domain package, its handler construction in app wiring, and the `/events` route are deleted; no dangling references
- [ ] The machines API doku no longer documents an events endpoint
- [ ] `docs/backend-conventions.md` names `streaming.Begin` + `Run` as the single entry point for NDJSON endpoints and points at the module tests as the invariant's home
- [ ] Deleting `ErrorHandler` breaks no test — proving it was pass-through dead weight

## Blocked by

- 250-2: Container logs through the orchestration
- 250-3: Quadlet logs through the orchestration
- 250-4: Image pull and auto-update through Begin

## Technical notes

- Gated on the migration slices only for the conventions ruling: the doc must describe the converged end state, not a mid-migration promise. The deletions themselves are mechanically independent and can be lifted out first if the slices stall.
- Route deletion is an API surface change (404 instead of 501) for an endpoint no client can have depended on.
