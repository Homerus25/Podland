---
title: Detach cached connection contexts from the dialing request
parent: engine seam - hand each domain its podman engine at the connection seam
date: 2026-09-09
status: completed
labels:
  - ready-for-agent
  - backend
  - connections
  - bugfix
  - testing
author: vibe-agent
---

## Parent

Engine seam epic (280-0-engine-seam-master). This slice fixes a live
connection-lifetime bug the design review surfaced; it gates the tracer slice
so the engine work does not build on broken lifetimes.

## What to build

A machine connection dialed by one HTTP request keeps working for every later
request that hits the Connection Cache. Today the cached connection context is
a value-child of the cold-dialing request's context (the fork's
`NewConnectionWithHandle` derives it with `context.WithValue` from the ctx it
is handed), and net/http cancels a request's context the moment its handler
returns. The stored context therefore dies with the request that dialed it,
and the next cache hit should fail its podman call immediately. In-process
tests cannot see this: no real server lifecycle runs.

The fix: the dial continues to honor the caller's context (a canceled caller
cancels the dial), but the context that gets cached is detached with
`context.WithoutCancel`, so connection-context lifetime equals connection
lifetime - governed by release/refcount, LRU eviction, idle cleanup, and
RemoveMachine, exactly as CONTEXT.md already describes.

`WithoutCancel` drops cancellation and deadline but keeps values, so the
bindings' client/version/machine-mode keys the podman calls rely on survive.

## Acceptance criteria

- [x] A regression test pins "the cached connection context outlives the
      dialing request": cancel the dialing parent after dial, then a second
      `WithConnection` still hands out a usable (non-canceled) context; the
      test fails against the old behavior
- [x] The dial itself still honors the caller's context - a canceled caller
      cancels a slow dial
- [x] Existing manager tests (dial dedup, eviction, idle cleanup,
      RemoveMachine teardown) stay green
- [x] A live diagnostic ran: two real requests through the running server to
      one machine, before and after the fix, with the outcome noted in this
      ticket
- [x] The fix lands regardless of the diagnostic's outcome - a
      non-reproducing diagnostic means something masks the hazard and that
      masking becomes its own follow-up finding, recorded in this ticket

## Blocked by

None - can start immediately

## Technical notes

- Verified with a standalone Go repro (net/http + a cached
  `context.WithValue` child): after the dialing request's handler returns, the
  child reads `context canceled` and a call through it would fail immediately.
- The manager currently passes the caller's ctx into the dial
  (`acquire` -> `dial` -> the fork's `NewConnectionWithHandle`); on cache hit
  the stored context is served unchanged. `ProbeMachine` already dials with
  `context.Background()` and is therefore the (ironically) safe path; detaching
  at dial time makes every path uniform.
- Keep the detach at one place in the connection stack so the lifetime rule
  has a single owner.

## Outcome

Implemented 2026-09-09. The detach lives in `acquire` (manager.go), the one
place every cached connection passes through, so both the production dial and
any injected test dial get the same lifetime rule.

Live diagnostic (integration harness: real podman machine, backend in-process
via httptest, one registered machine; three sequential real `GET
/api/machines/{id}/volumes` requests):

- Before the fix: request 1 (cold dial) 200; requests 2 and 3 (cache hits)
  failed 500 `INTERNAL_ERROR` with `Get "http://d/v6.1.0/libpod/volumes/json":
  context canceled` - exactly the predicted hazard, reproduced live.
- After the fix: all three requests 200; cache hits answer in ~1 ms vs ~94 ms
  for the cold dial.

Follow-up finding (the hazard reproduced, but its wire shape is its own
observation): a dead cached context surfaced as a 500 `INTERNAL_ERROR`
(`endpoint envelope: unexpected error`), not 502 `CONNECTION_FAILED` - a raw
`context canceled` from a canceled podman HTTP call does not classify as a
connection failure at the seam. With the detach that path becomes unreachable
for request-driven cancellation, but any future source of dead-cached-context
cancellation would mask as a 500 again. Recorded here per the ticket's
masking-becomes-a-follow-up rule; a separate ticket can decide whether
canceled-context errors crossing a domain service should map to 502.
