---
title: Conventions and CONTEXT.md record the URL owner
parent: 410-machine-api-url
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - frontend
  - refactoring
  - docs
author: vibe-agent
---

## Parent

`.scratch/410-machine-api-url/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

With every machine-scoped URL flowing through the builder, the written
record catches up so the next agent inherits the owner instead of the
drift: the frontend conventions name `machineApiPath`/`machineApiUrl`
as the single owner of machine-scoped request URLs — services route
every such path through them, fetch/download URLs come from the
service-level exports, and importing `apiBaseUrl` outside
`shared/utils/api` is off-limits — and `CONTEXT.md` gains the C11
decision entry in the style of the log-stream-kit entry.

## Acceptance criteria

- [ ] `docs/frontend-conventions.md` names the builder pair as the owner of machine-scoped request URLs (the encode-every-segment rule included) and states that stream/download URLs are consumed from the service-level exports, never re-derived in hooks or components
- [ ] The conventions no longer stop at the generic "encode path segments" phrasing where the builder now decides that
- [ ] `CONTEXT.md` gains the decision entry in the style of the log-stream-kit entry, ending with "Settled 2026-09-15 (architecture-review candidate C11)"
- [ ] No code changes in this slice

## Blocked by

- `410-machine-api-url-01-builder-and-retrofit`
