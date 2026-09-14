---
title: The log-stream kit and its pinned suites, with containers as the tracer adapter
parent: 380-log-stream-kit
date: 2026-09-13
status: open
labels:
  - ready-for-agent
  - backend
  - streaming
  - containers
  - testing
author: vibe-agent
---

## Parent

`.scratch/380-log-stream-kit/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The log-stream kit lands in `internal/streaming` with exactly the surface
the parent decided — `LogEvent`, `LogEventWriter`, `LogStreamOp`,
`RunLogStream` — plus the suites that pin its invariants once. The
containers domain converts as the tracer adapter: its `streamLogs`
method, `logStreamTerminalEvent` helper, `logEventWriter`, and
`ContainerLogEvent` die; the handler's follow branch becomes one
`RunLogStream` call behind the query-parse/dispatch ritual it keeps; the
service swaps its callback type and wraps it in the two kit writers. A
containers logs integration test joins the suite for parity with
quadlets. For a user nothing changes — same NDJSON events, same
timestamps, same terminal behavior — but from this slice on the
containers follow stream is served by the one kit, and the next
log-streaming domain starts at zero.

## Acceptance criteria

- [ ] `internal/streaming` exports `LogEvent` with today's JSON tags (`timestamp`, `level`, `message`); the emitted wire shape is byte-identical to today's containers events
- [ ] `internal/streaming` exports `LogEventWriter` (an `io.Writer` configured with a level and an `onEvent` callback): trims the trailing newline, skips empty lines, skips spaceless lines instead of panicking, parses the timestamp with `time.RFC3339Nano` (both `Z` and offset forms), and re-emits it as `time.RFC3339` UTC
- [ ] `internal/streaming` exports `LogStreamOp{Since, Tail, Stream}` and `RunLogStream(w, r, op)` with the parent-decided signature; `ErrConnectionFailed` is the baked-in error default with no override field
- [ ] `RunLogStream` behavior: a `Begin` failure returns silently; a clean stream emits every event then exactly one terminal `INFO` `STREAM_ENDED`; a returned `*AppError` maps to one `"CODE: message"` `ERROR` event; a cancelled context maps to one `WARN` event; a non-AppError cause maps to `ErrConnectionFailed` and is logged once by the kit, never surfaced raw
- [ ] Kit suites pin all of the above once, including the exactly-one-terminal invariant and the writer's line handling
- [ ] Containers: `streamLogs`, `logStreamTerminalEvent`, `logEventWriter`, and `ContainerLogEvent` are deleted; the `Logs` handler keeps query parse, the query-error `web.Handle` fallback, `containerId` extraction, follow dispatch, the buffered branch with `MISSING_CONTAINER_ID`, and its slog line, then calls `RunLogStream` with a closure over `GetLogsWithStream`
- [ ] Containers service: `GetLogsWithStream`'s callback is `func(streaming.LogEvent)`; it constructs the two `LogEventWriter` instances (stdout `INFO`, stderr `ERROR`) around the callback and hands them to the podman `Logs` call; the param guard and error mappings (404, cancellation) are unchanged
- [ ] The containers handler suite keeps Success, MissingID, StreamingHappyPath, NonStreamingUnchanged, and QueryErrorWithFollowRidesJSONEnvelope; the `WithSince`, `WithTail`, `InvalidSince`, and `InvalidTail` tests are deleted as kit-pinned
- [ ] The containers service suite keeps its error-mapping tests; `EmitsEventsPerLine` is deleted as writer-owned
- [ ] `Test_Integration_Containers_Logs` is added under `test/integration/`, mirroring the quadlets logs integration coverage (buffered fetch plus follow stream against the real podman machine, run-scoped-tag isolation model)
- [ ] The backend suite is green and touched files pass `gofmt` (including the build-tagged integration file, which `go fmt` skips)

## Blocked by

- None (can start immediately)

## Technical notes

- The spaceless-line skip is a behavior fix, not just dedup: today's containers writer indexes past a one-element split on a spaceless line and would panic mid-stream.
- Do not touch the quadlets domain in this slice; it migrates in `380-log-stream-kit-02`.
