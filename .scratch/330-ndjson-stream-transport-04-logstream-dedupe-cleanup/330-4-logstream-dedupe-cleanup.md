---
title: "Log-follow onto the transport, apiBaseUrl dedupe, dead-code contract"
parent: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - shared
  - streaming
  - containers
  - quadlets
  - refactoring
author: vibe-agent
---

## Parent

NDJSON stream transport epic (330-0-ndjson-stream-transport-master) - read
its "Design decisions" section first. This slice is the contract phase of
the expand-contract migration: the last hand-rolled streaming fetch moves
onto the function layer, and everything the migration left dead is deleted.

## What to build

The shared log-follow helper keeps its exact signature but its body stops
hand-rolling the streaming fetch: it calls the transport function and does
only what is genuinely its own - filtering values to log events, warning and
skipping the rest. Container and quadlet log following (the LogViewer flow)
behave exactly as today for a user: tail fetch, follow toggle, live lines,
connection teardown on toggle-off or close.

The API base URL constant, currently declared four times (both operation
hooks and both log API services), is exported once from the shared api
utils and every copy collapses onto it. The generic stream-event consumer
that the pull hook no longer uses is deleted along with its tests; the
stream-event types and guard stay, still used by the pull feature and the
stream test fixtures.

## Acceptance criteria

- [ ] `followLogStream` keeps its exact signature and resolves/rejects
      semantics; its body is a transport call plus the log-event filter
      (valid events forwarded, everything else warned and skipped)
- [ ] `isLogEvent` and `parseLogErrorEvent` are unchanged; the LogViewer,
      its log source interface, and both log API services are unchanged in
      behavior
- [ ] The api base URL is exported from the shared api utils and the four
      local declarations (both operation hooks, both log API services) are
      gone
- [ ] The generic stream-event consumer is deleted along with its tests,
      with no remaining importers; the stream-event types and shape guard
      remain
- [ ] The log-follow suite and LogViewer suite run with zero modifications
      and pass; the full frontend suite stays green

## Blocked by

- 330-3-auto-update-adapter (the dead-code deletions are only safe once
  both adapters have migrated)

## Technical notes

- An aborted fetch rethrowing raw is what the log-follow caller expects:
  the viewer distinguishes aborts from failures via its own signal check,
  so the transport's raw-abort rule preserves its behavior unchanged.
- The blob download helper is deliberately untouched (epic decision 3);
  if its preamble starts to hurt, that is a future candidate, not this
  slice.
