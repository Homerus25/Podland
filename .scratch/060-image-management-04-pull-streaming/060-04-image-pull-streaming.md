---
title: Image Pull — streaming progress + abort (POST /api/machines/{id}/images/pull)
parent: 060-image-management
date: 2026-07-30
status: completed
labels:
  - completed
  - images
  - backend
  - streaming
  - podman-integration
author: vibe-agent
---

## Parent

060-image-management — Implement Image Management endpoints using official podman implementation

## What to build

Upgrade the image pull endpoint to **stream progress** as newline-delimited JSON (NDJSON) and support **client-initiated abort**, per the API contract. This builds on the basic working pull from slice 060-03 and converts its response mechanism from a single JSON object to a streamed series of events.

The user calls `POST /api/machines/{id}/images/pull`. The backend responds with `Content-Type: application/json` and writes one JSON object per line:

- `progress` events carrying download/layer state (`current`, `total`, `status`, `message`)
- a single terminal `complete` event on success
- a terminal `error` event (e.g. `OPERATION_CANCELLED`) on failure, after which the connection closes

If the client closes the connection, the backend detects the disconnect and stops the pull, emitting a final `error` event with code `OPERATION_CANCELLED`.

This slice covers: progress-event DTOs, an `operationId`, the streaming handler, client-disconnect detection, and tests for the streaming/abort behavior.

## Acceptance criteria

- [ ] `Handler.Pull` sets `Content-Type: application/json` and flushes each event to the client as NDJSON (one JSON object per line, immediately flushed)
- [ ] Progress-event DTOs exist and match the contract: a wrapper with `operationId`, `type` (`progress`|`complete`|`error`), `machineId`, optional `state` (`current`/`total`/`status`/`message`), and `timestamp`; the `error` variant carries `error.code`/`error.message`
- [ ] Each pull request is assigned a unique `operationId`
- [ ] `progress` events are emitted as the pull proceeds; a single `complete` event is emitted on success
- [ ] On pull failure the handler emits a terminal `error` event with an appropriate code and closes the connection
- [ ] When the client disconnects (`r.Context().Done()` / closed response writer), the handler stops the operation and emits a terminal `error` event with code `OPERATION_CANCELLED` (best-effort) and cleans up
- [ ] The connection to the machine is released (`Release`) on all exit paths (success, error, abort) via `defer`
- [ ] Unit tests cover: successful stream emits progress + complete; service failure emits terminal error; client cancellation triggers `OPERATION_CANCELLED` and release
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` all pass

## Blocked by

- 060-03-image-pull-basic (provides the working `Service.Pull` to stream over)

## Technical notes

- Streaming event shapes: `doku/description_api.md` lines 749-807 (progress, complete, and error variants)
- The podman tunnel `ImagePullOptions` exposes a `Writer` (`opts.Writer`, set via `WithProgressWriter`) — `~/repos/podman/pkg/domain/infra/tunnel/images.go:124`. Wire a writer/adapter that converts podman progress output into the NDJSON progress events. If podman progress is not easily surfaced through `Writer`, fall back to emitting periodic `progress` events with a `status` message and ensure a clean `complete`/`error` terminal — the key requirement is a streamed, flush-per-event response with correct terminal events.
- Use `http.Flusher` to flush after each line: `if f, ok := w.(http.Flusher); ok { f.Flush() }`
- Detect client disconnect via `<-r.Context().Done()` in a goroutine and cancel the podman operation with a `context.WithCancel` derived from the request context
- Connection-release-on-all-paths: `defer connManager.Release(machineID)` immediately after a successful `GetConnection`
- Keep the `ImagePullRequest` DTO and validation from slice 060-03 unchanged
