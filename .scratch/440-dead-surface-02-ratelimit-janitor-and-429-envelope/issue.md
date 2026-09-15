---
title: Rate-limit janitor: lazy eviction plus one 429 envelope
parent: 440-dead-surface
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - middleware
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/440-dead-surface/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The rate limiter stops advertising a janitor nobody wired and starts
actually bounding its memory, and the two 429s speak one vocabulary.
`RateLimiter.Cleanup` — a zero-caller infinite sleep-loop — is deleted;
instead `Allow` performs an amortized sweep: when the last sweep is
older than the window, entries whose window started more than 2×window
ago are evicted. For a user, the only visible change is that a
throttled authenticated request now gets the structured
`RATE_LIMIT_EXCEEDED` error body (as `doku/description_api.md` has
always promised) instead of an empty one, with the same
`X-RateLimit-*` headers as before.

## Acceptance criteria

- [ ] `RateLimiter.Cleanup` is gone; no background goroutine or stop mechanism is introduced anywhere; `cmd/server/main.go` and `internal/app` are untouched.
- [ ] `Allow` evicts lazily: a sweep runs at most once per window (tracked via a last-sweep timestamp on the store), deleting entries whose window started more than 2×window ago. No per-call full scan of the map.
- [ ] A test pins the eviction: with a short window, entries that age past 2×window disappear from the store after a subsequent `Allow` triggers the sweep, and the map no longer holds them (e.g. observed via `GetRemaining` resetting or by exposing nothing new — white-box on the store is fine).
- [ ] The per-user `RateLimitMiddleware` 429 answers the `RATE_LIMIT_EXCEEDED` envelope via `apperrors.RespondWithError` with the request ID from context — same shape as `AuthRateLimitMiddleware` — and still sets the `X-RateLimit-*` headers first so they survive the envelope write.
- [ ] `AuthRateLimitMiddleware` behavior is unchanged.
- [ ] Existing rate-limit tests stay green; the suite grows one eviction pin and one per-user 429 body/headers pin (or the existing middleware test extends to assert the body).
- [ ] No doku or CONTEXT.md change: the envelope was already the documented contract, and CONTEXT.md §14's "fixed window per user" description remains true.
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` pass in `backend/`; touched files pass `gofmt`.

## Blocked by

- None (can start immediately)

## Technical notes

- The headers-before-envelope ordering matters: `setRateLimitHeaders` must run before `RespondWithError` writes the body, mirroring the auth middleware's ordering.
- The sweep cadence and the 2×window retention are the epic's decision 2 — amortized O(active keys) work per window, not per request.
