---
title: Close the buffered-logs endpoint-envelope bypass (architecture review C2)
parent: none
date: 2026-09-12
status: open
labels:
  - needs-triage
  - backend
  - refactoring
author: vibe-agent
---

## Problem Statement

The buffered (follow=false) log endpoints in two domains bypass the endpoint
envelope: each re-implements, by hand, exactly the ritual `web.Handle` owns —
identity extraction, the AppError→envelope mapping, the 500 fallback, and the
JSON encode+status. That is a leak across the envelope's seam, duplicated
across containers and quadlets, and the two copies have already drifted (the
quadlets copy silently ignores a malformed `tail`, the containers copy
rejects it). The `follow=true` sibling already crosses the streaming envelope
(`streaming.Begin` + `streaming.Run`), so the buffered branch is the only
unsealed path.

## Solution

Buffered logs cross `web.Handle` like every other JSON endpoint, and the
log-query parsing (`follow`/`since`/`tail`) becomes one shared unit instead of
two hand-rolled copies. Per docs/backend-conventions.md the log-stream kit is
only at two copies — below the third-copy threshold — so the bypass is the
friction, not the copy count; this epic closes the bypass without touching the
streaming path.

> Update 2026-09-13: the streaming path this epic deliberately left was extracted by `.scratch/380-log-stream-kit` (architecture review C7), which supersedes the below-threshold scoping above.
> Update 2026-09-15: the binary paths this ruling left out (volumes export/populate) were sealed by `.scratch/400-binary-envelope` (architecture review C8).

## Decisions (from the design review — do not relitigate)

1. **Parser home**: the shared log-query parser lives in `internal/streaming`.
   The query vocabulary is the stream-request language (`follow` selects the
   streaming envelope), and both log handlers already import `streaming` for
   the follow path.
2. **Parser shape**: split by failure semantics — `ParseFollow(r)` returns a
   bool and never fails (invalid values mean `false`, today's behavior);
   `ParseLogWindow(r)` returns `(since, tail, *AppError)` with a **strict**
   contract: malformed `tail` or `since` is a 400 `INVALID_TAIL_PARAM` /
   `INVALID_SINCE_PARAM`, messages unified to "use RFC3339".
3. **Dispatch shape**: the handler parses once, then the rule is *only a clean
   follow request goes to the streaming envelope* — a query error takes the
   `web.Handle` path even when `follow=true` was requested, so every buffered
   JSON response (including query-validation failures) crosses the single
   envelope seam and no handler ever writes an error envelope by hand.
4. **Identity contract**: missing userID/machineID on the buffered path become
   the envelope's 500 INTERNAL_ERROR with a loud log — same as every other
   envelope endpoint and same as the follow path's `streaming.Begin`. The
   subtree middleware guarantees both in production, so this only affects
   miswired routes. Route-owned IDs (`MISSING_CONTAINER_ID`,
   `MISSING_QUADLET_ID`) stay 400 as handler-owned `NewAppError`s returned
   from the closure.
5. **Logging**: the happy-path slog ritual in the buffered path is dropped;
   failure logging is centralized in `web.Handle` (unexpected errors) —
   matching the envelope-era handlers.

Frontend impact: none. It always sends numeric `tail` values, so the strict
contract breaks no caller.

## Slices

- `340-buffered-logs-envelope-01-parser-containers` — shared parser in
  `internal/streaming` + containers buffered logs cross the envelope (tracer
  bullet through parser, handler, tests, doku).
- `340-buffered-logs-envelope-02-quadlets` — quadlets buffered logs cross the
  same seam; `tail` becomes strict in the second domain, unifying the query
  contract.
