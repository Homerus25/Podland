--- 
title: Route Every Streaming Endpoint Through One Orchestration
parent: N/A
date: 2026-09-08
status: ready-for-agent
labels:
  - streaming
  - backend
  - epic
  - high-priority
author: vibe-agent
---

## What to build

Converge all four NDJSON streaming endpoints (container logs, quadlet logs, image pull, quadlet auto-update) onto one orchestration in the `streaming` module, so "cancel on disconnect, forward progress, write exactly one terminal event" has one home instead of four divergent hand-rolled shapes. Delete the dead code the review flagged alongside it.

Origin: architecture review 2026-09-07 ("Route every streaming endpoint through one orchestration", rated Strong). All design decisions were settled in a grilling session on 2026-09-08; the decision ledger lives in the slice tickets.

## Settled decisions (from the grilling session)

1. All four NDJSON endpoints converge; no exceptions left behind.
2. Log-stream terminal errors use the containers wire format: a log-shaped event with the code embedded in the message (`"CONTAINER_NOT_FOUND: …"`). The frontend parser already speaks it; quadlet streams become parseable by it.
3. `streaming.Begin(w, r)` mirrors the `web.Handle` envelope: headers, NDJSON writer, identity extraction, early errors. Missing identity is a wiring bug: 500 `INTERNAL_ERROR` + loud log (replaces today's hand-copied 401 checks).
4. Log streams use `Result = struct{}`; a clean stream end fires the complete event: `INFO` `STREAM_ENDED`. Exactly-one-terminal then holds in every exit direction and fixes containers' hang-until-disconnect wart.
5. `Run` owns the AppError→event mapping: config carries the domain's default sentinel; the handler's error-event builder receives already-mapped `(code, message, cancelled)`, never the raw error.
6. Non-AppError failures surface the default sentinel's generic message; `Run` logs the raw cause once, centrally (the cause-logging ruling keeps causes out of wire payloads).
7. Auto-update's 10-minute write deadline stays in its handler (operation-specific fact, not orchestration).
8. Orchestration invariants are pinned once in the `streaming` package tests; per-domain handler tests shrink to one happy-path + one terminal-error streaming test each.
9. The pass-through `ErrorHandler` dies in favour of `errors.ToAppError`; the unimplemented `events` SSE domain, route, and its doku section are deleted outright.

## Child Issues (implementations in dependency order)

1. **250-1**: Streaming module — Begin, shared error mapping, config change - `250-streaming-orchestration-01-streaming-module/250-1-streaming-module.md`
2. **250-2**: Container logs through the orchestration - `250-streaming-orchestration-02-containers-logs/250-2-container-logs-streaming.md`
3. **250-3**: Quadlet logs through the orchestration - `250-streaming-orchestration-03-quadlets-logs/250-3-quadlet-logs-streaming.md`
4. **250-4**: Image pull and auto-update through Begin - `250-streaming-orchestration-04-pull-autoupdate/250-4-pull-and-autoupdate-through-begin.md`
5. **250-5**: Deletions and the conventions ruling - `250-streaming-orchestration-05-deletions/250-5-errorhandler-events-deletions.md`

## Dependencies

```
250-1 (streaming module)
    ↓
250-2 (container logs) ─┐
250-3 (quadlet logs) ───┼→ 250-5 (deletions + ruling)
250-4 (pull/auto-update)┘
```

250-2, 250-3, 250-4 can run in parallel once 250-1 lands.

## Success Criteria

- [ ] All four NDJSON endpoints cross `streaming.Begin` + `streaming.Run`; no hand-rolled select loops, callback writers, or identity rituals remain in handlers
- [ ] Exactly one terminal event in every exit direction: success, operation error, client disconnect, and clean stream end
- [ ] Missing identity on a streaming route responds 500 with a loud log, identical to the JSON envelope's ruling
- [ ] Log-stream failures are `"CODE: message"` log events on both domains; quadlet failures parse as `ApiError` in the frontend
- [ ] Container log streams emit `STREAM_ENDED` on clean podman EOF instead of hanging until disconnect
- [ ] `ErrorHandler` and the `events` domain are deleted; nothing references them
- [ ] `docs/backend-conventions.md` names the streaming module the single entry point for NDJSON endpoints

## Current State

- `images.Pull` and `quadlets.AutoUpdate` already use `streaming.Run` but hand-copy the entry ritual (identity checks with 401s, headers, writer, `STREAMING_NOT_SUPPORTED`).
- `containers.streamLogs` re-creates `Run`'s select loop line for line, maps errors via sentinel pointer-equality, and hangs until disconnect on clean EOF.
- `quadlets.GetLogs` (follow mode) writes from the service callback with an `atomic.Bool` guard and a hand-rolled wait-for-goroutine-exit to avoid write-after-return corruption; its error events are `map[string]any` shapes the frontend cannot parse.
- `errors.ErrorHandler` is a pass-through used by auth, users, and app wiring.
- `GET /api/machines/{id}/events` is documented in doku but implemented as a 501 stub with `any`-typed dependencies.
