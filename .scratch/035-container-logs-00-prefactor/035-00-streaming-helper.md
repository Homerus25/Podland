---
title: Prefactor — extract shared NDJSON streaming helper
parent: 035-container-logs
date: 2026-08-02
status: completed
labels:
  - completed
  - containers
  - backend
  - refactor
  - streaming
  - testability
author: vibe-agent
---

## Parent

035-container-logs — Implement Container Logs endpoint (backend) against the podman service API

## What to build

Extract the generic HTTP/NDJSON streaming mechanics out of `internal/domain/images/handler.go` into a reusable package so that container logs (035-02) and quadlet logs (070-07) can both lean on it instead of duplicating the wiring. This is a "make the change easy, then make the easy change" prefactor — no new endpoint behavior is added.

Today the only streaming code lives inline in the image-pull handler (`ImageHandler.Pull`): it sets the streaming response headers, asserts the `http.Flusher`, marshals+writes+flushes one JSON line at a time, and watches the request context for client disconnect. These mechanics are generic; only the event payload (`PullProgressEvent`) is pull-specific.

This slice should:

- Create a new `internal/streaming` package exposing the **primitives**, not the orchestration:
  - `SetNDJSONHeaders(w http.ResponseWriter)` — sets the headers image-pull currently sets by hand (`Content-Type`, `Cache-Control: no-cache`, `Connection: keep-alive`, `Transfer-Encoding: chunked`). Keep `Content-Type: application/json` to match the existing pull handler and the `doku/description_api.md` contract (which declares NDJSON under `application/json`).
  - `NewNDJSONWriter(w http.ResponseWriter) (*NDJSONWriter, error)` — asserts the `http.Flusher` and returns a small helper, or returns an error the handler can map to a `STREAMING_NOT_SUPPORTED` response. Expose a `Write(v any) error` that marshals, `Fprintln`s, and `Flush`es (i.e. the current `writeAndFlushEvent` generalized to `any`).
  - `DisconnectChan(ctx context.Context) <-chan struct{}` — returns a channel that is closed when `ctx` is done (the current inline goroutine pattern).
- Refactor `internal/domain/images/handler.go` `Pull` to use the new package: replace the hand-rolled header block, the `w.(http.Flusher)` assertion, the disconnect goroutine, and the local `writeAndFlushEvent` with calls into `internal/streaming`. The pull-specific event loop (`select` on progress/result/error/disconnect) and the `PullProgressEvent` types **stay** in the images package.

Behavior of image pull must be unchanged — this is a pure relocation. Existing image handler/service tests are the safety net.

## Acceptance criteria

- [ ] `internal/streaming` package exists with the header/flusher/write/disconnect primitives described above
- [ ] `internal/domain/images/handler.go` `Pull` is refactored to use `internal/streaming`; the local `writeAndFlushEvent` helper is removed or reduced to a thin wrapper
- [ ] No pull-specific types (`PullProgressEvent`, `EventState`, `EventError`) move out of the images package
- [ ] Image-pull behavior is unchanged: existing `handler_test.go` / `service_test.go` pass without modification
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] `go test ./...` passes

## Blocked by

None - can start immediately

## Technical notes

- The code being extracted currently lives inline in `internal/domain/images/handler.go` (`Pull`, lines ~95–245): the header block (~134–138), the `http.Flusher` assertion (~141–146), the disconnect goroutine (~149–153), and `writeAndFlushEvent` (~235–245).
- Keep `Content-Type: application/json` (not `application/x-ndjson`) to stay consistent with both the existing pull handler and `doku/description_api.md:1091-1093`, which documents NDJSON streaming under `application/json`.
- Extract only the clearly-duplicated mechanics. The `select` event loops differ between pull (progress/result/error/disconnect) and logs (line/done/error/disconnect), so leave orchestration in each handler.
- The package is internal (`internal/streaming`) because it is backend HTTP plumbing, not a public API. The empty `pkg/utils/` directory is unrelated and should not be used.
