---
title: Conventions and CONTEXT.md record the decode ritual
parent: 430-request-decode
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - docs
author: vibe-agent
---

## Parent

`.scratch/430-request-decode/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

With every subtree handler on `web.Decode`, the written record catches
up so the next agent inherits the one-owner ruling instead of the
copy-the-sibling ritual: the backend conventions' JSON-envelope bullet
names `web.Decode` as the owner of decode+validate (the volumes handler
stays the reference shape), the duplication section's threshold count
moves from three crossings to four, and `CONTEXT.md` gains the C9
decision entry. The `doku/` files are verified unchanged in substance —
the wire contract never moved.

## Acceptance criteria

- [ ] `docs/backend-conventions.md`'s JSON-envelope bullet names `web.Decode`: a handler is `web.Decode` + URL params + service call inside one `web.Handle` closure; decode and validation errors belong to `web.Decode`'s vocabulary and are never hand-minted per handler; pre-stream sites respond the returned AppError themselves (images Pull the reference).
- [ ] The same file's duplication section records the fourth threshold crossing: the decode+validate ritual became `web.Decode` ("crossed three times" becomes four, the new extraction joins `web.Handle`, `testutils.Perform`, and `internal/podmanerr` as the reference).
- [ ] `CONTEXT.md` gains a backend decision entry for the decode ritual in the style of the C8 binary-envelope entry, ending with "Settled 2026-09-15 (architecture-review candidate C9)".
- [ ] doku checked against the shipped endpoints: every touched endpoint's documented 400 codes still match (`INVALID_REQUEST` / `VALIDATION_FAILED`); no doku hunk is needed, and any mismatch found is fixed in the same change.
- [ ] No code changes in this slice.

## Blocked by

- `430-request-decode-01-seam-and-migration` (the conventions text declares a reference that must already be true in the code)
