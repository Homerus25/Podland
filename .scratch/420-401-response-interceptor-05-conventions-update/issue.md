---
title: Conventions record the 401 owner
parent: 420-401-response-interceptor
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - docs
author: vibe-agent
---

## Parent

`.scratch/420-401-response-interceptor/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The frontend conventions name the 401 owner so the next feature suite
doesn't re-grow a 401 story. One addition to `docs/frontend-conventions.md`:
401s are owned by the shared session policy — the axios response
interceptor and the stream transport via `handleUnauthorized` — the
refresh/replay/seam semantics are pinned once in the shared suites (the
api unit suite, the stream transport suite, the App-level wiring test),
feature suites never pin 401 behavior, and file downloads are out of
scope.

## Acceptance criteria

- [ ] `docs/frontend-conventions.md` states the ownership rule as above: shared policy owns 401s, semantics pinned once in shared suites, feature suites never pin 401 behavior, downloads excluded
- [ ] No code changes in this slice — docs only

## Blocked by

- `420-401-response-interceptor-04-stream-coverage`

## Technical notes

- `CONTEXT.md` already settled this decision ("global 401 interceptor for token expiration") and no endpoint contract changed, so neither it nor `doku/` need edits.
