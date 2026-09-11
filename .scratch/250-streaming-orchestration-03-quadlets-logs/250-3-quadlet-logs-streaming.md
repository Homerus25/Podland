--- 
title: Quadlet Logs Through the Orchestration
parent: route every streaming endpoint through one orchestration
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - quadlets
  - handlers
  - frontend
author: vibe-agent
---

## Parent

Part of the "route every streaming endpoint through one orchestration" epic (`250-streaming-orchestration/250-master-streaming-orchestration.md`).

## What to build

Migrate the quadlet follow-mode log endpoint from its callback-writer shape onto `streaming.Begin` + `streaming.Run`, and align its terminal-event wire format with the shared log-stream contract:

- The handler enters via `Begin` (identity 500 ruling replaces its hand-copied 401 checks) and drives `GetLogsWithStream` through one `Run` config with `Result = struct{}` and the `CONNECTION_FAILED` default sentinel.
- The `atomic.Bool` write-error guard, the disconnect channel, and the hand-rolled wait-for-goroutine-exit (the shutdown-timeout select that guards against write-after-return corruption) are all deleted — under `Run`, event writes happen in the orchestration loop, never from the service goroutine, so the hazard is impossible by construction.
- Terminal error events switch from today's untyped `{code, message}` maps to the shared log-stream shape — a log event with `level: ERROR` (`WARN` when cancelled) and message `"CODE: human message"`:

```
{"timestamp": "...", "level": "ERROR", "message": "QUADLET_NOT_FOUND: Quadlet not found"}
```

  This is the fix the frontend has been waiting for: quadlet stream failures now parse through the existing `parseLogErrorEvent` heuristic and surface as real `ApiError`s instead of raw ERROR log lines.
- A clean stream end emits the `INFO` `STREAM_ENDED` terminal event.

## Acceptance criteria

- [ ] Follow-mode handler is a `Begin` + `Run` config with no callback writer, no `atomic.Bool`, no disconnect channel, and no shutdown-timeout wait
- [ ] Terminal errors are log-shaped `"CODE: human message"` events (`ERROR` level; `WARN` for cancelled); the ad-hoc `code`-field map shape is gone
- [ ] Clean stream end emits exactly one `INFO` `STREAM_ENDED` event
- [ ] Missing identity responds 500 `INTERNAL_ERROR` (existing handler tests updated from the 401 expectations)
- [ ] Frontend: the shared log-stream helper's comment no longer credits only the containers handler, and a parser test pins that quadlet terminal errors (e.g. `QUADLET_NOT_FOUND: …`) parse into an `ApiError`
- [ ] Handler streaming tests shrink to happy path, one terminal-error case, and the clean-end case; integration log tests that pinned the old map shape are updated
- [ ] The quadlets API doku documents the terminal-event format and `STREAM_ENDED`

## Blocked by

- 250-1: Streaming module — Begin, shared error mapping, config change

## Technical notes

- The buffered (non-follow) logs endpoint keeps its shape; only follow mode crosses the orchestration.
- No frontend behaviour change beyond error surfacing: the log viewer renders `STREAM_ENDED` and terminal lines as ordinary log events.
