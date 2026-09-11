---
title: Container logs endpoint (streaming / follow)
parent: 035-container-logs
date: 2026-08-02
status: completed
labels:
  - completed
  - containers
  - backend
  - feature
  - podman-integration
  - streaming
author: vibe-agent
---

## Parent

035-container-logs — Implement Container Logs endpoint (backend) against the podman service API

## What to build

Add the streaming variant of the container-logs endpoint on top of the non-streaming tracer bullet (035-01). When `follow=true`, the handler switches from returning a single `{id, logs}` JSON object to streaming newline-delimited JSON: `Content-Type: application/x-ndjson` semantically (set via `internal/streaming.SetNDJSONHeaders`, which keeps `application/json` per the codebase convention), and each line is a JSON object `{"timestamp": "<ISO8601>", "level": "<INFO|WARN|ERROR|…>", "message": "<line>"}`.

Log lines arrive from podman via the `StdoutWriter`/`StderrWriter` callbacks on `ContainerLogsOptions`. The service exposes a streaming variant of `GetLogs` that takes a callback (one log-line event at a time), mirroring how `ImageService.PullWithProgress` takes a `progressWriter` callback — the handler then writes each event through `internal/streaming` and flushes.

If the client disconnects mid-stream, the handler must emit a final `OPERATION_CANCELLED` event (best-effort) and stop, matching the image-pull behaviour. `follow=false` continues to return the `{id, logs}` object from 035-01 unchanged.

### Cancellation seam (decision required during implementation)

Podman's `ContainerLogs` (`~/repos/podman/pkg/domain/infra/tunnel/containers.go:550`) runs `containers.Logs(ic.ClientCtx, …)` inside a goroutine driven by its **own internal** `context.Background()` select loop. That means cancelling the **handler's** request context does not automatically stop a `follow=true` stream — the loop only ends when podman's internal goroutine completes or `ic.ClientCtx` (the SSH-tunnel context) is cancelled. The implementer must decide how to make client-disconnect actually unwind (e.g. tie the request context into the connection context, or return from the handler on disconnect and accept best-effort goroutine cleanup). Document the chosen approach in the issue/PR.

## Acceptance criteria

- [ ] `follow=true` streams NDJSON lines `{"timestamp", "level", "message"}` rather than returning `{id, logs}`
- [ ] `follow=false` returns `{id, logs}` exactly as in 035-01 (no regression)
- [ ] Streaming uses the `internal/streaming` primitives from 035-00 (headers + NDJSON writer + disconnect channel)
- [ ] Client disconnect during `follow=true` results in an `OPERATION_CANCELLED` terminal event (best-effort) and the handler stops writing
- [ ] `level` is derived from whether the line came from `StdoutWriter` (INFO) or `StderrWriter` (ERROR/WARN) — pick a simple, documented rule
- [ ] `since`/`tail` continue to work in streaming mode
- [ ] `CONTAINER_NOT_FOUND` and `CONNECTION_FAILED` are still returned (as JSON errors before the stream begins, or as a terminal NDJSON error event once streaming has started — be consistent with image-pull)
- [ ] Unit tests cover: streaming happy-path (multiple lines), non-streaming unchanged, container-not-found, and cancellation on client disconnect (use a cancelable context + the `internal/streaming.DisconnectChan`)
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` are green

## Blocked by

- 035-01-container-logs-non-streaming

## Technical notes

- Streaming pattern to mirror: `internal/domain/images/service.go` `PullWithProgress` (callback-based) + `internal/domain/images/handler.go` `Pull` (NDJSON event loop with disconnect detection). The `internal/streaming` package from 035-00 now holds the shared primitives.
- Podman `ContainerLogs` writes one line at a time to `opts.StdoutWriter`/`opts.StderrWriter`. Wrap each writer so each `Write` becomes one `{timestamp, level, message}` event passed to the service callback. Note podman may include a trailing newline per line — trim/normalize as needed.
- `entities.ContainerLogsOptions.Follow = true` is what makes podman keep the stream open. `Timestamps = true` asks podman to prepend timestamps to each line (alternative to generating `timestamp` yourself).
- Cancellation: see the "Cancellation seam" note above. The image-pull handler cancels its pull context on disconnect and lets podman return; here the propagation into podman's internal loop is indirect — verify with a manual/integration test that a dropped client actually stops the stream on the remote machine.
- This slice directly informs quadlet logs (070-07), which resolves a quadlet to a container and then does the same streaming. Keep the log-event shape and the service callback signature compatible so 070-07 can reuse them.
