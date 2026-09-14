---
title: Give follow-mode log streams one kit (architecture review C7)
parent: none
date: 2026-09-13
status: open
labels:
  - needs-triage
  - backend
  - refactoring
author: vibe-agent
---

## Problem Statement

The follow-mode log-stream kit exists twice and the copies have already
drifted. Containers (`streamLogs` + `logEventWriter`) and quadlets
(`GetLogs`'s streaming twin + `quadletLogEventWriter`) carry identical
dispatch rituals and terminal-event helpers, but two timestamp parse
layouts (containers parses a literal-`Z` layout with no space guard, so a
spaceless podman line would panic the writer; quadlets parses
`time.RFC3339Nano` behind a space guard) and two field-for-field identical
DTO types (`ContainerLogEvent` ≡ `QuadletLogEvent`). Issue 340 deliberately
left the streaming path at two copies ("below the third-copy threshold"),
but that threshold exists to catch drift, and drift is what we have: the
copies can no longer be changed in one place.

The base kit is already shared: `internal/streaming` owns `Begin`/`Run`,
`ParseLogWindow`/`ParseFollow`, and the NDJSON writer. What remains
duplicated is everything log-specific above that base.

## Solution

One log-stream kit in `internal/streaming` owns the follow ritual end to
end: the shared `LogEvent`, the one line writer, and a `RunLogStream`
entry point that wires `Begin`+`Run`, the `STREAM_ENDED` complete event,
the `"CODE: message"` terminal events, and the connection-failed default.
Each domain stays a thin adapter that keeps only what is genuinely its
own: query parse with the JSON-envelope fallback (issue 340's dispatch
ruling, untouched), route-ID extraction, the buffered branch with its
`MISSING_*_ID` 400, and resolving its target (quadlet to container name)
inside the service. The wire contract does not change; `doku/` is
untouched.

## Decisions (from the design session — do not relitigate)

1. **Kit scope: the follow branch only.** `RunLogStream` owns the
   follow=true ritual — slog entry, `Begin`, `Run` wiring, the terminal
   `INFO` `STREAM_ENDED`, `"CODE: message"` error events (`ERROR`, `WARN`
   when cancelled), and the error default. Handlers keep query parse, the
   query-error `web.Handle` fallback, ID extraction, follow dispatch, the
   buffered branch, and one slog line naming their target; the
   `streamLogs` methods and `logStreamTerminalEvent` helpers are deleted
   from both domains.
2. **Op seam (a type shape encodes this more precisely than prose):**

   ```go
   type LogStreamOp struct {
       Since  time.Time
       Tail   int64
       Stream func(ctx context.Context, userID, machineID string, onEvent func(LogEvent)) error
   }

   func RunLogStream(w http.ResponseWriter, r *http.Request, op LogStreamOp)
   ```

   `ErrConnectionFailed` is baked in as the error default — no override
   knob; both domains share it today and a second vocabulary earns the
   knob.
3. **`LogEvent` is kit-owned.** `{timestamp, level, message}` with today's
   JSON tags. `ContainerLogEvent` and `QuadletLogEvent` are deleted and
   their references swapped (handler config, service callbacks, tests).
   The JSON wire shape is unchanged, so `doku/` and the frontend are
   untouched (the frontend treats `timestamp` as an opaque string).
4. **One `LogEventWriter`, quadlets' behavior.** An `io.Writer` the
   services hand to the podman `Logs` call, one instance per stream with
   the level configured (stdout `INFO`, stderr `ERROR`): trims the
   newline, skips empty lines, skips spaceless lines (fixing containers'
   latent index panic), parses timestamps as `time.RFC3339Nano` (accepts
   `Z` and offset forms — a superset of containers' layout), and re-emits
   `time.RFC3339` UTC.
5. **Services construct the writers.** `GetLogsWithStream` keeps its
   signature with the callback type swapped to `func(streaming.LogEvent)`;
   the service wraps the callback in the two kit writers and hands them
   to the engine's `Logs` call. The podman call stays domain-side;
   quadlets' `resolveForLogs` resolution is untouched.
6. **Test split (pinned once).** New kit suites pin `RunLogStream` (happy
   path, exactly-one-terminal including `STREAM_ENDED`, AppError to
   `CODE: message` `ERROR`, cancellation to `WARN`, non-AppError to
   `ErrConnectionFailed` + logged once by the kit) and `LogEventWriter`
   (trim, skip-empty, space guard, both timestamp forms, RFC3339 re-emit).
   Per-domain handler suites keep exactly what the handler still owns:
   Success, MissingID, StreamingHappyPath, NonStreamingUnchanged,
   QueryErrorWithFollowRidesJSONEnvelope. Deleted: the four param twins
   (`WithSince`, `WithTail`, `InvalidSince`, `InvalidTail` — parse is
   pinned once in `streaming`'s query suite), the service
   `EmitsEventsPerLine` twins, and `TestQuadletLogEventWriter`. Service
   error-mapping tests (404, cancellation) and quadlets' name-resolution
   tests stay. A containers logs integration test is added for parity
   with quadlets; quadlets' two existing integration tests must pass
   unchanged.
7. **Drift ruling (supersedes 340's scoping).** The third-copy threshold
   exists to catch drift; drift between copies triggers extraction even
   below the third copy. Issue 340's "without touching the streaming
   path" scoping served its epic; this epic is that path's extraction.

## Slices

- `380-log-stream-kit-01-kit-and-containers-tracer` — the kit (`LogEvent`,
  `LogEventWriter`, `LogStreamOp`, `RunLogStream`) with its pinned suites,
  containers converted as the tracer adapter, plus the containers logs
  integration test.
- `380-log-stream-kit-02-quadlets-and-test-thinning` — the quadlets twin
  dies; both domains' suites reach their final thinned shape; quadlets'
  integration tests pass unchanged.
- `380-log-stream-kit-03-conventions-update` — the backend conventions
  record the kit as the log-stream reference and the drift amendment;
  `CONTEXT.md` gains the C7 decision entry; issue 340 gains the
  cross-link; `doku/` verified unchanged.
