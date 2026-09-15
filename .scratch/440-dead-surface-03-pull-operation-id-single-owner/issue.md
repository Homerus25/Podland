---
title: One owner for the image pull operation ID
parent: 440-dead-surface
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - images
  - refactoring
author: vibe-agent
---

## Parent

`.scratch/440-dead-surface/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The image pull handler becomes the single, honest owner of the
operation ID, and the service stops maintaining a stamp that never
survives and an entry point nobody calls. Today the handler mints a
uuid and overwrites `event.OperationID` on every progress event, so the
service-side `pull-<nano>` stamp is dead on arrival (and its format
contradicts the documented uuid); the non-progress `Pull` wrapper on
the service has zero production callers. After this slice every
streamed pull event — progress, complete, and error — carries exactly
one ID, minted once, in the handler. Nothing changes on the wire.

## Acceptance criteria

- [ ] `generateOperationID` and the `progressAdapter`'s `operationID` field are gone from the images service; the adapter emits progress events with the zero-value operation ID, which the handler callback stamps (as it already does — handler code is unchanged).
- [ ] `ImageService.Pull` (the nil-callback wrapper) is deleted; `PullWithProgress` remains the only pull entry point.
- [ ] The three service tests that drove `Pull` are ported to `PullWithProgress(..., nil)` and stay green (success, pull-failure mapping, cancellation → `ErrOperationCancelled`).
- [ ] The handler is untouched: it still mints the uuid, stamps every event, and feeds the complete/error events — doku's `operationId: string (uuid)` stays true for progress, complete, and error events.
- [ ] No frontend change: the streaming transport's operationId contract is identical.
- [ ] No doku change.
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` pass in `backend/`; touched files pass `gofmt`.

## Blocked by

- None (can start immediately)

## Technical notes

- If `NewProgressEvent`'s operation-ID parameter becomes awkward with the adapter no longer stamping, trimming that parameter (handler-side construction paths unaffected) is in scope — the event shape on the wire must not change.
