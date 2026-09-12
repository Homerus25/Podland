---
title: The image pull hook as the first transport adapter
parent: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - images
  - streaming
  - refactoring
  - testing
author: vibe-agent
---

## Parent

NDJSON stream transport epic (330-0-ndjson-stream-transport-master) - read
its "Design decisions" section first. This slice converts the smallest
operation hook, the image pull, as the proof adapter for
`useNdjsonOperation`.

## What to build

The image pull hook stops hand-rolling its streaming fetch and lifecycle.
Starting a pull, cancelling it, a second pull superseding the first, the
terminal complete/error handling, and the error mapping all become the
transport module's job. What stays in the feature is genuinely its own: the
pull request's URL and body, the progress state derived from `progress`
events, and the images list invalidation on completion.

For a user, pulling an image behaves exactly as today: progress renders
live, cancelling stops the operation without an error, a superseded pull's
late events change nothing, a registry failure shows its envelope, and a
completed pull refreshes the images list. The existing pull suite runs
untouched and green - it is the behavior-preserving gate for this slice.

## Acceptance criteria

- [ ] `usePullImage` keeps its exact public shape (`isPulling`, `progress`,
      `error`, `pull`, `cancel`) and its call sites are unchanged
- [ ] No hand-rolled fetch, AbortController, finish guard, supersede check,
      or error-event/HTTP-error mapping remains in the hook; the transport
      module owns all of it
- [ ] The hook keeps its progress-event guard and derives `progress` only
      from valid `progress` events; malformed values are warned and skipped
      as today
- [ ] A `complete` event still invalidates the machine's images queries
      exactly once; a dry run concept does not exist here, so no conditional
- [ ] An `error` event still surfaces its code and message (generic fallback
      unchanged), mapped onto the hook's `error`
- [ ] The existing pull suite runs with zero modifications and passes; the
      full frontend suite stays green

## Blocked by

- 330-1-module-and-suite (the module and its pinned suite)

## Technical notes

- The hook's local API base URL constant stays for now; the shared export
  lands in slice 4 together with the other copies.
- The suite already pins supersede, abort-without-error, and malformed-line
  behavior through MSW; keep it as the gate - thinning is slice 5's job,
  only after the whole migration is green.
