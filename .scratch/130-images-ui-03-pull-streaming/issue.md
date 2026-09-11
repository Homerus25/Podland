---
title: Images UI — pull image with streaming NDJSON progress and cancel
parent: 130-images-ui
date: 2026-08-21
status: open
labels:
  - ready-for-agent
  - frontend
  - images
  - feature
author: vibe-agent
---

## Parent

`.scratch/130-images-ui/issue.md`

## What to build

Add a pull flow to the Images tab: the user enters an image reference (e.g. `nginx:latest`) and the pull runs against the backend's NDJSON progress stream, showing live status text and a progress bar (current/total bytes) with a cancel button. This is the frontend's first streaming consumer and establishes the streaming-fetch pattern: plain `fetch` (not axios) with an `AbortController`, a `usePullImage` hook owning stream state (latest progress event, isPulling, abort) that later streaming flows (container logs, PRD 140) can reuse, and a stream-mocking test helper later log viewers reuse.

Stream handling per the backend contract: parse NDJSON lines into progress/complete/error events (`operationId`, `state.current/total/status/message`); on `complete` invalidate the images query so the new image appears; on `error` route the envelope error into the global error display; malformed NDJSON lines are logged and skipped, never fatal; chunks split across reads are reassembled. Cancelling aborts the fetch so the backend actually cancels the operation (`OPERATION_CANCELLED` path). One pull at a time per form instance; starting a new pull aborts the previous one.

## Acceptance criteria

- [ ] Pull form accepts an image reference and starts a streaming pull via `fetch` + `AbortController`
- [ ] Live UI shows status text and a progress bar with current/total bytes, plus a working cancel button
- [ ] Cancel aborts the stream such that the reader is closed and the stream is cancelled (backend `OPERATION_CANCELLED` path)
- [ ] On `complete`, the images query is invalidated and the list refreshes without manual reload
- [ ] On `error`, the envelope error (code + message) renders in the global error display
- [ ] Malformed NDJSON lines are logged and skipped; chunks split across reads reassemble correctly
- [ ] Only one pull runs per form instance; starting a new pull aborts the previous one
- [ ] `usePullImage` and the stream-mocking test helper are reusable, non-image-specific
- [ ] Vitest + RTL tests with mocked `fetch` returning a `ReadableStream` covering: progress updates, complete → invalidation, error envelope, abort/cancel, malformed lines, split chunks

## Blocked by

- `.scratch/130-images-ui-01-image-list/issue.md`
