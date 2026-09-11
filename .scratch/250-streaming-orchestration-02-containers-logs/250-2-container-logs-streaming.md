--- 
title: Container Logs Through the Orchestration
parent: route every streaming endpoint through one orchestration
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - containers
  - handlers
author: vibe-agent
---

## Parent

Part of the "route every streaming endpoint through one orchestration" epic (`250-streaming-orchestration/250-master-streaming-orchestration.md`).

## What to build

Migrate the container follow-mode log endpoint off its hand-rolled select loop onto `streaming.Begin` + `streaming.Run`, so the handler shrinks to config plus event vocabulary:

- The handler enters via `Begin` (headers, writer, identity — including the new 500-on-missing-identity ruling replacing its hand-copied 401 check) and drives the stream with one `Run` config: operation = `GetLogsWithStream` with `Result = struct{}`, default sentinel = `CONNECTION_FAILED`.
- The hand-rolled select loop, the disconnect channel plumbing, and the sentinel pointer-equality branches (`ErrContainerNotFound`, `ErrOperationCancelled` special cases) are deleted — the shared mapping produces the same wire outcome because the codes travel inside the sentinels.
- Terminal error events keep the existing wire format users see today (a log-shaped event, `level: ERROR`, message `"CODE: human message"`); cancelled streams use `level: WARN` as today. The frontend contract is unchanged.
- A clean podman stream end (container stopped) now emits the `INFO` `STREAM_ENDED` terminal event instead of hanging until the client disconnects — the exactly-one-terminal invariant finally holds in this exit direction.
- The write-after-return hazard stays dead by construction: event writes happen inside `Run`'s loop, never from the service goroutine.

## Acceptance criteria

- [ ] Follow-mode handler is a `Begin` + `Run` config with no select loop, no disconnect channel, and no error-classification branches
- [ ] Happy-path log events are byte-identical to today on the wire
- [ ] Terminal errors are log-shaped `"CODE: human message"` events (`ERROR` level; `WARN` for cancelled) — unchanged from today
- [ ] Clean stream end emits exactly one `INFO` `STREAM_ENDED` event and the handler returns promptly
- [ ] Missing identity responds 500 `INTERNAL_ERROR` (existing handler tests updated from the 401 expectation)
- [ ] Handler streaming tests shrink to: happy path, one terminal-error case, and the clean-end `STREAM_ENDED` case — orchestration mechanics are not re-tested here
- [ ] The containers API doku reflects the `STREAM_ENDED` terminal event

## Blocked by

- 250-1: Streaming module — Begin, shared error mapping, config change

## Technical notes

- Today's `CONTAINER_NOT_FOUND` / `OPERATION_CANCELLED` / `CONNECTION_FAILED` outcomes must emerge from the shared mapping without domain branches: verify each against the pinned module mapping table rather than re-implementing the classification.
- The non-streaming (buffered) logs endpoint is untouched by this ticket.
