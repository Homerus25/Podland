---
title: Conventions and CONTEXT.md record the log-stream kit and the drift ruling
parent: 380-log-stream-kit
date: 2026-09-13
status: open
labels:
  - ready-for-agent
  - backend
  - refactoring
  - docs
author: vibe-agent
---

## Parent

`.scratch/380-log-stream-kit/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

With both domains on the kit, the written record catches up so the next
agent inherits the new reference instead of the retired one: the backend
conventions' NDJSON ruling points log streams at `streaming.RunLogStream`
instead of the containers handler, the duplication ruling gains the drift
trigger that justified this extraction, `CONTEXT.md` gains the C7
decision entry mirroring C1's, and issue 340 gains a cross-link noting
its streaming-path scoping is superseded. The `doku/` files are verified
unchanged in substance — the wire contract never moved.

## Acceptance criteria

- [ ] `docs/backend-conventions.md`'s NDJSON streaming ruling names `streaming.RunLogStream` as the log-stream reference: the kit owns the follow ritual (terminal events, `STREAM_ENDED`, the error default) and a log handler is query parse + dispatch + two closures; the images pull handler remains the progress-stream reference
- [ ] The "Duplication across domains" section records the amendment: drift between copies triggers extraction even below the third copy — the threshold exists to catch drift, not to count copies — citing this epic as the precedent
- [ ] `CONTEXT.md` gains a backend decision entry for the log-stream kit in the style of the C1 stream-transport entry, ending with "Settled 2026-09-13 (architecture-review candidate C7)"
- [ ] `.scratch/340-buffered-logs-envelope/issue.md` gains a one-line cross-link under its Solution noting the streaming path it deliberately left was extracted by `380-log-stream-kit`
- [ ] `doku/description_containers_api.md` and `doku/description_quadlets_api.md` are checked against the shipped endpoints: params, statuses, NDJSON event shape, and terminal-event contract all still match; no doku hunk is needed, and if any mismatch is found it is fixed in the same change
- [ ] No code changes in this slice

## Blocked by

- `380-log-stream-kit-02-quadlets-and-test-thinning` (the conventions text declares a reference that must already be true in both domains)
