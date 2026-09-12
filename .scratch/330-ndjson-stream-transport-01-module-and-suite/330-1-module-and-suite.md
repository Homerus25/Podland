---
title: The stream transport module and its pinned suite
parent: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - shared
  - streaming
  - testing
author: vibe-agent
---

## Parent

NDJSON stream transport epic (330-0-ndjson-stream-transport-master) - read
its "Design decisions" section first; this slice lands the two-layer module
those decisions describe and pins its contract once. Nothing consumes the
module yet (the expand phase of the expand-contract migration); the suite is
the proof the slice is done.

## What to build

One new shared streaming module exports two things. `streamNdjson` is the
streaming fetch every endpoint currently hand-rolls: it injects the Bearer
credential, turns non-ok responses into the displayable error envelope and
everything else that fails into the generic sentinel, leaves abort errors
raw for the lifecycle owner to interpret, and pumps every parsed NDJSON
value to the caller without judging its shape. `useNdjsonOperation` is the
operation lifecycle both mutation-stream hooks hand-rolled: starting a new
operation supersedes the previous one, terminal `complete`/`error` events
end the run exactly once, an error event or transport failure surfaces as
the hook's error state, an abort never does, and unmounting tears the
connection down.

The module's suite becomes the one place the transport invariants live:
auth, error mapping, abort, supersede, and terminal rules, each pinned
through MSW exactly as the feature suites drive their endpoints today. For
a user nothing changes in this slice - no production call site moves - and
the next streaming endpoint can be built on the module immediately after.

## Acceptance criteria

- [ ] `streamNdjson(url, init, onValue)` sends the Bearer header when a
      token is held, and omits it when none is
- [ ] `Content-Type: application/json` is set only when `init.body` is
      present; a GET (log-follow shape) sends no content type
- [ ] A non-ok response rejects with the envelope from
      `getApiErrorFromResponse` (client errors pass their code through,
      internal ones collapse to the generic sentinel)
- [ ] A response without a body rejects the generic sentinel; a clean
      stream end resolves
- [ ] An aborted fetch or stream read rethrows the raw error; a non-abort
      fetch or read failure rejects as the generic ApiError
- [ ] Every parsed value reaches `onValue`, malformed lines are warned and
      skipped, and events split across reads are reassembled (the shared
      parser's guarantees still hold underneath)
- [ ] `useNdjsonOperation` returns `{ isRunning, error, start, cancel }`;
      `start({ url, init?, onEvent })` returns void
- [ ] Calling `start` while a run is in flight aborts the previous request
      (its signal is aborted) and its late events and terminal side effects
      never reach the caller or disturb the newer run's state
- [ ] A value with a string `operationId` and type `complete` finishes the
      run; type `error` finishes it and sets `error` from the event's code
      and message, falling back to the generic sentinel; a second terminal
      event cannot rerun those side effects; all values, terminal ones
      included, reach `onEvent` while the run is current and not aborted
- [ ] `cancel` aborts the in-flight run and leaves `error` null; unmount
      aborts too; `isRunning` is true exactly between start and terminal
- [ ] The module suite drives everything through MSW with the existing
      stream response helpers; the full frontend suite stays green

## Blocked by

None (can start immediately)

## Technical notes

- Reuse the shared stream test utilities (ndjson, chunked, live, abortable
  response builders and the event fixtures); the fixtures are typed to the
  existing StreamEvent shape, and the open non-terminal vocabulary means
  raw-chunk overrides cover foreign event types.
- The suite needs no QueryClient - invalidation stays feature-side by
  design; hook tests use `renderHook`.
- The AbortSignal cross-realm relay in the test setup applies: create
  controllers normally, never hand-build signals that bypass the relay.
