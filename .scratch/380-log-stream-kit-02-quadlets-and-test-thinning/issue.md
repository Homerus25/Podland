---
title: The quadlets twin dies: migration and final test thinning
parent: 380-log-stream-kit
date: 2026-09-13
status: open
labels:
  - ready-for-agent
  - backend
  - streaming
  - quadlets
  - testing
author: vibe-agent
---

## Parent

`.scratch/380-log-stream-kit/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The quadlets domain migrates onto the kit the tracer slice landed: its
`streamLogs` twin, `logStreamTerminalEvent`, `quadletLogEventWriter`, and
`QuadletLogEvent` die; the `GetLogs` handler keeps its query-parse and
dispatch ritual and calls `RunLogStream` with a closure over
`GetLogsWithStream`; the service swaps its callback type and wraps it in
the two kit writers around its podman `Logs` call. With both domains on
the kit, the drift the review flagged is structurally impossible — one
timestamp layout, one event type, one terminal vocabulary — and both
domains' suites reach their final thinned shape. For a user nothing
changes; quadlets' two existing integration tests must pass unmodified,
proving the wire contract held.

## Acceptance criteria

- [ ] Quadlets: `streamLogs`, `logStreamTerminalEvent`, `quadletLogEventWriter`, and `QuadletLogEvent` are deleted; no reference to a domain-local log-event type remains in either domain
- [ ] The `GetLogs` handler keeps query parse, the query-error `web.Handle` fallback, `quadletId` extraction, follow dispatch, the buffered branch with `MISSING_QUADLET_ID`, and its slog line, then calls `RunLogStream` with a closure over `GetLogsWithStream`
- [ ] The quadlets service: `GetLogsWithStream`'s callback is `func(streaming.LogEvent)`; it constructs the two `LogEventWriter` instances around the callback; `resolveForLogs` and its name-resolution precedence (explicit `ContainerName`, pod container, `systemd-<basename>`) are untouched
- [ ] The quadlets handler suite keeps Success, MissingID, StreamingHappyPath, NonStreamingUnchanged, and QueryErrorWithFollowRidesJSONEnvelope; the `WithSince`, `WithTail`, `InvalidSince`, and `InvalidTail` tests are deleted as kit-pinned
- [ ] The quadlets service suite keeps its error-mapping and name-resolution tests; `EmitsEventsPerLine` and `TestQuadletLogEventWriter` are deleted as writer-owned
- [ ] `Test_Integration_Quadlets_Logs` and `Test_Integration_Quadlets_Logs_ExplicitName` pass without modification
- [ ] The backend suite is green and touched files pass `gofmt`

## Blocked by

- `380-log-stream-kit-01-kit-and-containers-tracer` (the kit surface and its suites must exist to migrate onto)

## Technical notes

- Quadlets' writer behavior is the one the kit adopted, so this migration is deletion, not adaptation — anything that needs new kit behavior means the kit got the wrong shape; raise it against the parent instead of working around it locally.
