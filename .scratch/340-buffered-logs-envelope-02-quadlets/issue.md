---
title: Quadlets buffered logs cross the endpoint envelope; strict tail everywhere
parent: 340-buffered-logs-envelope
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - backend
  - quadlets
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/340-buffered-logs-envelope/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

Quadlet buffered logs (`follow=false`) cross `web.Handle` through the same
seam the containers slice built: the handler parses once with the shared
`streaming` parser, only a clean follow request proceeds to the streaming
envelope, and the hand-rolled `bufferedLogs` copy is deleted.

This slice also unifies the query contract in the second domain: a malformed
`tail` is no longer silently ignored (today the quadlets copy swallows it and
serves all lines) but rejected with the same 400 `INVALID_TAIL_PARAM` error
envelope containers already answers — making both log endpoints speak one
strict, documented query language. `MISSING_QUADLET_ID` stays a handler-owned
400 AppError from the closure; identity misses become the envelope's 500.

## Acceptance criteria

- [ ] Buffered quadlet logs respond through `web.Handle` with status 200; the hand-rolled `bufferedLogs` path is deleted
- [ ] Invalid `tail` now answers 400 `INVALID_TAIL_PARAM` (the existing test that pins the lenient fall-back-to-all-lines behavior is flipped to the strict contract)
- [ ] Invalid `since` still answers 400 `INVALID_SINCE_PARAM` with the unified "use RFC3339" wording (via the shared parser)
- [ ] `MISSING_QUADLET_ID` still yields 400 with that code, now as a closure AppError
- [ ] The per-domain missing-machine-ID handler test is deleted (identity is the envelope's 500)
- [ ] Existing handler tests for success, since/tail propagation, and all `follow=true` streaming tests still pass unchanged (except the flipped and deleted ones)
- [ ] Quadlets API doku updated: add `INVALID_TAIL_PARAM` to the 400 codes, drop the "buffered mode also answers MISSING_MACHINE_ID" note, `since` wording says RFC3339
- [ ] Both log endpoints (containers + quadlets) now speak an identical query contract — no behavioral divergence remains between the two domains
- [ ] `go vet` / `go test ./...` green in the backend module

## Blocked by

- `340-buffered-logs-envelope-01-parser-containers` (introduces the shared parser both handlers consume)
