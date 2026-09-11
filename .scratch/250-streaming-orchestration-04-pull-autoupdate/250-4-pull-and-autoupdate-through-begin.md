--- 
title: Image Pull and Auto-Update Through Begin
parent: route every streaming endpoint through one orchestration
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - images
  - quadlets
  - handlers
author: vibe-agent
---

## Parent

Part of the "route every streaming endpoint through one orchestration" epic (`250-streaming-orchestration/250-master-streaming-orchestration.md`).

## What to build

Complete the convergence: the two endpoints that already used `streaming.Run` (image pull, quadlet auto-update) drop their hand-copied entry ritual and enter through `streaming.Begin` like everyone else:

- Both handlers replace their hand-rolled identity checks (401 on missing userID, 400 on missing machineID), header setup, writer creation, and `STREAMING_NOT_SUPPORTED` early error with one `Begin` call — including the new missing-identity ruling (500 `INTERNAL_ERROR` + loud log), aligning them with the JSON envelope.
- Their error-event builders shrink to pure event vocabulary: they receive the module-mapped `(code, message, cancelled)` and stamp it into their typed events. The domain-specific default sentinels (`IMAGE_PULL_FAILED`, `CONNECTION_FAILED` for auto-update) travel via the config's default-sentinel field.
- Wire behaviour is preserved: pull and auto-update events (progress, complete, error with structured `code` fields) keep their existing typed vocabularies and shapes — this ticket changes the entry path, not the event contract.
- Auto-update's 10-minute write-deadline extension stays in its handler, just before `Begin`: it is an operation-duration fact (registry pulls), not orchestration.

## Acceptance criteria

- [ ] Both handlers start with `Begin`; no hand-copied identity checks, headers, writer creation, or unsupported-streaming error handling remain
- [ ] Missing identity on either endpoint responds 500 `INTERNAL_ERROR` with a loud log (tests updated from the 401/400 expectations)
- [ ] Progress, complete, and error events on both endpoints are byte-identical to today on the wire
- [ ] Error-event builders contain no error classification — only typed-event construction from `(code, message, cancelled)`
- [ ] Auto-update still extends its write deadline to 10 minutes before streaming begins
- [ ] The images handler comment claiming pull "keeps its own shape" is replaced by the shared-orchestration note
- [ ] Existing pull and auto-update stream tests pass with only identity-expectation updates

## Blocked by

- 250-1: Streaming module — Begin, shared error mapping, config change

## Technical notes

- This is the contract step of the convergence: after it lands, all four NDJSON endpoints share one entry path and one mapping, and the epic's per-endpoint differences are down to config and event vocabulary alone.
