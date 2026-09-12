---
title: Conventions update and test thinning
parent: "NDJSON stream transport - one owner for every streaming fetch's auth, abort, supersede, and terminal rules"
date: 2026-09-12
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - shared
  - streaming
  - documentation
  - testing
author: vibe-agent
---

## Parent

NDJSON stream transport epic (330-0-ndjson-stream-transport-master) - read
its "Design decisions" section first. This slice records the ruling so the
next streaming endpoint starts from knowledge, and thins the now-duplicated
test coverage - only after the whole migration is green.

## What to build

The frontend conventions gain the stream-transport ruling in the same
spirit as the ResourceList and MutationDialog rulings: every streaming
endpoint rides the transport module's two layers - operations on the
operation hook, everything else on the streaming function - and a feature
keeps only its event handlers, shape guards, and invalidation choices. No
feature hand-rolls a streaming fetch, an abort lifecycle, or a terminal
guard again; the next streaming endpoint starts at zero.

With the migration complete and green, the transport invariants the module
suite pins come out of the feature suites, which keep their domain
assertions. The user-visible behavior is exactly what it was; only the
maintenance surface shrinks.

## Acceptance criteria

- [ ] frontend-conventions.md documents the stream-transport ruling: which
      layer serves which streaming shape, what a feature owns (event
      handlers, guards, invalidation), and that transport invariants are
      pinned once in the module suite
- [ ] CONTEXT.md records the module decision alongside the ResourceList and
      MutationDialog rulings
- [ ] The pull suite drops its module-owned re-pins (supersede,
      abort-without-error, malformed-line handling) and keeps its domain
      assertions: request shape, progress exposure, invalidation on
      complete, error-event and non-ok envelope routing
- [ ] The auto-update suite drops its transport re-pins and keeps its
      domain assertions: line accumulation, report capture, the dry-run
      invalidation skip, connection-error mapping
- [ ] The log-follow suite keeps its log-event filtering and error-event
      parsing cases and drops its transport-level cases (auth, non-ok,
      no-body, abort)
- [ ] The full frontend suite is green and finishes within its recorded
      runtime budget

## Blocked by

- 330-4-logstream-dedupe-cleanup (thinning is only safe once the whole
  migration is green)

## Technical notes

- Follow the ResourceList precedent: one adapter-wiring smoke case per
  feature (e.g. pull's non-ok envelope routing) may stay even where the
  module also pins it - the line is "domain surfaces through the adapter",
  not double-covering the transport.
- The conventions file's test section is the natural home for the ruling's
  pointer; keep it to what a new feature slice needs to start correctly.
