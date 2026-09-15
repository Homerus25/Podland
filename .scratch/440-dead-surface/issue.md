---
title: Delete the dead surface (architecture review C10)
parent: none
date: 2026-09-15
status: open
labels:
  - needs-triage
  - backend
  - refactoring
author: vibe-agent
---

## Problem Statement

Three surfaces advertise behaviour nobody gets:

1. **Dead mock twins** — `MockPodmanConnectionService` and
   `MockPodmanConnection` in testutils satisfy no production interface
   (every method speaks `any`); the only caller in the entire repo is
   testutils' own self-test. Roughly 45 lines of mock maintenance that
   mocks nothing real.
2. **Unwired rate-limit janitor** — `RateLimiter.Cleanup` has zero
   callers and is an infinite sleep-loop that could never stop, so even
   wiring it would mean inventing server lifecycle. Meanwhile the
   `entries` map genuinely grows unbounded (the auth limiter keys on
   client IP). While there: the per-user 429 carries
   `X-RateLimit-*` headers but an empty body, while the auth variant
   answers the structured `RATE_LIMIT_EXCEEDED` envelope that
   `doku/description_api.md` documents as the contract for every 429.
3. **Double-stamped pull operation ID** — the image pull handler mints
   a uuid and overwrites `event.OperationID` on every event, so the
   service-side `pull-<nano>` stamp never survives; and the
   non-progress `ImageService.Pull` wrapper has zero production callers
   (only its own tests use it — the handler goes straight to
   `PullWithProgress`).

## Solution

Deletion. The twins and the service-side stamping die; the janitor is
replaced by lazy eviction inside `Allow` (no goroutine, no lifecycle);
both 429s speak the one documented envelope. Roughly 70 lines leave the
codebase and every surviving line keeps a caller.

## Decisions (from the design session — do not relitigate)

1. **Delete the mock twins wholesale** — both types, their three
   constructors, and the self-test `TestMockPodmanConnection`. Nothing
   production-side can notice. (The `sshexec` and `bindings` imports
   stay: `MockSSHExec` and `mockConnectionContext` still use them.)
2. **Janitor: delete `Cleanup`, evict lazily** — no background
   goroutine, `main.go` untouched. `Allow` performs an amortized sweep:
   when the last sweep is older than the window, delete entries whose
   window started more than 2×window ago. The map stays bounded by
   keys active in the last two windows.
3. **One 429 vocabulary** — the per-user `RateLimitMiddleware` answers
   the same `RATE_LIMIT_EXCEEDED` envelope as the auth variant
   (requestID from context). `X-RateLimit-*` headers stay in both.
   doku already documents this contract; no doku edit.
4. **Handler owns the pull operation ID** — the handler's uuid stays
   the single stamper across progress/complete/error (it must: it feeds
   the complete/error events too, and doku documents `operationId` as a
   uuid). The service-side stamping (`generateOperationID`, the
   adapter's `operationID` field) is deleted; the adapter emits the
   zero value and the handler callback fills it, exactly as it already
   does.
5. **Delete the production-dead `ImageService.Pull` wrapper** — port
   its three tests to `PullWithProgress(..., nil)`. Same spirit as the
   epic: a misleading entry point leaves the service.
6. **Three independent slices** — mock twins / rate limiter / pull
   operation ID touch disjoint areas (testutils, middleware, images)
   and block nothing of each other.
7. **Verification bar** — backend-only diff; `go build ./...`,
   `go vet ./...`, `go test ./...` all green in `backend/`; frontend
   untouched; doku and CONTEXT.md unchanged (CONTEXT.md §14 stays
   accurate: still fixed-window per-user; the wire contracts were
   already documented correctly — the code aligns to them).

## Vertical Slices

### 1. Delete the mock twins
**Issue**: `440-dead-surface-01-delete-mock-twins`
- testutils loses `MockPodmanConnectionService`,
  `MockPodmanConnection`, their constructors, and the self-test

**Status**: ready-for-agent
**Blocked by**: none

### 2. Rate-limit janitor and the one 429
**Issue**: `440-dead-surface-02-ratelimit-janitor-and-429-envelope`
- `Cleanup` dies, lazy eviction bounds the map, the per-user 429 gains
  the documented envelope

**Status**: ready-for-agent
**Blocked by**: none

### 3. One owner for the pull operation ID
**Issue**: `440-dead-surface-03-pull-operation-id-single-owner`
- service-side stamping and the dead `Pull` wrapper die; the handler's
  uuid is the only stamper

**Status**: ready-for-agent
**Blocked by**: none

## Acceptance criteria

- [ ] No dead surface remains: no type, method, or helper in the three
      touched areas lacks a production caller (self-referential tests
      excepted — they are deleted with their subjects)
- [ ] The rate-limit store's map is bounded by active keys (verified by
      test), with no background goroutine or server-lifecycle change
- [ ] Both 429 variants answer the `RATE_LIMIT_EXCEEDED` envelope with
      `X-RateLimit-*` headers
- [ ] Every streamed pull event carries the handler's uuid operation ID
      (progress, complete, and error) — wire contract unchanged
- [ ] Backend suite green: `go build ./... && go vet ./... && go test ./...`
- [ ] No frontend, doku, or CONTEXT.md changes
