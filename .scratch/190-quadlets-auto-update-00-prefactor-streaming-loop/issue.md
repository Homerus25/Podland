---
title: "Prefactor: extract reusable NDJSON streaming handler loop"
parent: 190-quadlets-auto-update
date: 2026-08-30
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - images
  - refactoring
  - streaming
author: vibe-agent
---

## Parent

`.scratch/190-quadlets-auto-update/issue.md` — Quadlet auto-update with live streaming output

## What to build

The image pull streaming endpoint currently orchestrates its NDJSON response inline in the handler: a goroutine running the service operation with a progress callback, channels for progress/results/errors, client-disconnect detection, and the terminal complete/error event writing. The upcoming quadlet auto-update endpoint needs exactly this orchestration and would otherwise copy ~90 lines of it.

Extract the orchestration into a reusable helper in the shared streaming support, generic over the progress-event type, the result type, and how a result becomes a terminal event. The helper owns: running the operation with the progress callback, forwarding events to the NDJSON writer, disconnect detection and cancellation, mapping errors (including context cancellation) to a terminal error event, and guaranteeing exactly one terminal event. Switch the image pull endpoint to the helper.

Behavior must be unchanged: the existing image pull streaming handler tests pass unmodified, emitting the same events in the same order. This slice adds no endpoint and changes no contract — it makes the auto-update endpoint cheap to build.

## Acceptance criteria

- [ ] A reusable streaming-run helper exists in the shared streaming support, generic over progress event, result, and terminal-event construction
- [ ] The helper enforces exactly one terminal event (complete or error) and cancels the operation on client disconnect
- [ ] The image pull streaming endpoint uses the helper; no inline channel/disconnect orchestration remains in that handler
- [ ] Existing image pull streaming handler tests pass unmodified (same events, same order, same cancellation semantics)
- [ ] `go build ./...` succeeds, `go vet ./...` is clean, existing tests pass

## Blocked by

None - can start immediately
