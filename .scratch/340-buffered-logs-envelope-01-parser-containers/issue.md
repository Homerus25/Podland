---
title: Log-query parser in streaming + containers buffered logs cross the endpoint envelope
parent: 340-buffered-logs-envelope
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - backend
  - containers
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/340-buffered-logs-envelope/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Container buffered logs (`follow=false`) stop hand-rolling the response
ritual and cross `web.Handle` like every other JSON endpoint, and the
log-query parsing becomes a shared, tested unit in `internal/streaming` that
this slice introduces and consumes end to end.

A caller sees: the happy path byte-identical to today (200 JSON with the
container ID and logs), invalid `since`/`tail` rejected with the same 400
error envelope as before, `follow=true` streaming completely unchanged — but
internally there is exactly one seam: no handler code extracts identities,
maps AppErrors, writes fallback 500s, or encodes JSON by hand anymore.

The parser: `ParseFollow` (lenient — invalid values simply mean `false`) and
`ParseLogWindow` (strict — malformed `since` or `tail` is a 400 AppError,
message wording unified to "use RFC3339"; defaults: zero time, tail -1).
The handler parses once; only a clean follow request proceeds to the
streaming envelope, while query errors ride the JSON envelope even when
`follow=true` was requested — so no code path ever writes an error envelope
by hand. `MISSING_CONTAINER_ID` remains a handler-owned 400 AppError
returned from the `web.Handle` closure; missing identity becomes the
envelope's 500 INTERNAL_ERROR. The per-request happy-path slog lines go
away (failure logging stays centralized in the envelope).

## Acceptance criteria

- [ ] `internal/streaming` exports `ParseFollow(r) bool` and `ParseLogWindow(r) (since, tail, *AppError)` per the parent's decision 2
- [ ] Table-driven tests in the streaming package pin: valid since/tail round-trip, defaults (zero time, tail -1), 400 `INVALID_SINCE_PARAM` / `INVALID_TAIL_PARAM` with "use RFC3339" wording, and follow leniency
- [ ] Buffered container logs respond through `web.Handle` with status 200; the hand-rolled `bufferedLogs` path is deleted
- [ ] Query errors answer through the JSON envelope even when `follow=true` is set (400 before the stream begins) — single-seam rule holds
- [ ] `MISSING_CONTAINER_ID` still yields 400 with that code, now as a closure AppError
- [ ] The per-domain missing-machine-ID handler test is deleted (identity is the envelope's 500, already pinned in the web package's own tests)
- [ ] Existing handler tests for success, since/tail propagation to the engine, invalid since/tail, and all `follow=true` streaming tests still pass unchanged (except the deleted one)
- [ ] The happy-path slog ritual (entry + success lines) is gone from the buffered path; unexpected-error logging remains via the envelope
- [ ] Containers API doku updated: drop the "buffered mode also answers MISSING_MACHINE_ID" note, `since`/`tail` wording says RFC3339
- [ ] `go vet` / `go test ./...` green in the backend module

## Blocked by

- None (can start immediately)
