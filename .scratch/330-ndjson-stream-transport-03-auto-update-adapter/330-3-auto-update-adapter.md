---
title: The quadlet auto-update hook as the second transport adapter
parent: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - quadlets
  - streaming
  - refactoring
  - testing
author: vibe-agent
---

## Parent

NDJSON stream transport epic (330-0-ndjson-stream-transport-master) - read
its "Design decisions" section first. This slice converts the second and
last operation hook, proving the module serves a stream whose non-terminal
vocabulary differs from pull's.

## What to build

The quadlet auto-update hook stops copy-pasting the image pull pattern. Its
run/cancel/supersede/terminal/error machinery moves to the transport
module; the feature keeps what is genuinely its own: the auto-update request
and its dryRun parameter, the `log`-event vocabulary with its local shape
guard, the accumulated output lines, the completion report, and the
invalidation that a dry run skips because it mutates nothing.

For a user, running an auto-update (dry or real) behaves exactly as today:
log lines stream live, the report renders on completion, a connection-level
failure shows its envelope as the connection error, cancelling stops
quietly, and a real run refreshes quadlets, containers, and images while a
dry run refreshes nothing. The existing suite runs untouched and green.

## Acceptance criteria

- [ ] `useQuadletAutoUpdate` keeps its exact public shape (`isRunning`,
      `dryRun`, `lines`, `report`, `connectionError`, `run`, `cancel`) and
      its call sites are unchanged
- [ ] No hand-rolled fetch, AbortController, finish guard, supersede check,
      or error mapping remains in the hook; the transport module owns it
- [ ] The `isAutoUpdateEvent` guard stays feature-side and receives raw
      values; `log` events append lines (stdout default preserved),
      malformed values are warned and skipped as today
- [ ] A `complete` event still captures the report and invalidates
      quadlets, containers, and images on a real run only - the dry-run skip
      stays feature logic with its invalidation spy intact
- [ ] The hook's `connectionError` maps the transport module's error state
      unchanged (terminal error events and HTTP failures alike)
- [ ] The existing auto-update suite runs with zero modifications and
      passes; the full frontend suite stays green

## Blocked by

- 330-2-pull-adapter (the chain keeps module evolution single-threaded;
  this slice is otherwise logically independent)

## Technical notes

- The hook's local API base URL constant stays for now; the shared export
  lands in slice 4.
- The transport module knows nothing of `log` events or reports - if this
  slice needs module changes beyond what slice 1 pinned, stop and revisit
  the epic's design decisions instead of widening the module ad hoc.
