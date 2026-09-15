---
title: Conventions and CONTEXT.md record the binary envelope
parent: 400-binary-envelope
date: 2026-09-15
status: open
labels:
  - ready-for-agent
  - backend
  - refactoring
  - docs
author: vibe-agent
---

## Parent

`.scratch/400-binary-envelope/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

With both endpoints on the seam, the written record catches up so the
next agent inherits the new reference instead of the bypass: the backend
conventions gain a third reference bullet naming `web.HandleStream` as
the seam for binary/multipart machine-scoped endpoints (volumes
export/populate the reference), `internal/web`'s package doc drops its
JSON-only phrasing, `CONTEXT.md` gains the C8 decision entry, and issue
340 gains a cross-link noting its remaining unsealed paths were closed
by this epic. The `doku/` files are verified unchanged in substance —
the wire contract never moved.

## Acceptance criteria

- [ ] `docs/backend-conventions.md` gains a "binary endpoints" reference bullet beside the JSON-envelope and NDJSON-streaming bullets: `web.HandleStream` is the seam for machine-scoped endpoints whose success is a byte stream or multipart upload rather than a DTO — the envelope owns identity (500 invariant), AppError mapping, and the 500 fallback; the handler owns headers, body ritual, and the success write; the volumes handler is the reference
- [ ] The JSON-envelope bullet's "exceptions that keep their own shape" no longer needs an export/populate carve-out — check the current wording and fold these two endpoints into the sealed set if they are called out
- [ ] `internal/web`'s package doc speaks of the endpoint envelope across JSON and binary paths, not JSON only; `HandleStream`'s doc comment mirrors `Handle`'s (identity invariant, unwrapping, fallback)
- [ ] `CONTEXT.md` gains a backend decision entry for the binary envelope in the style of the C7 log-stream-kit entry, ending with "Settled 2026-09-15 (architecture-review candidate C8)"
- [ ] `.scratch/340-buffered-logs-envelope/issue.md` gains a one-line cross-link noting the binary paths its ruling left out were sealed by `400-binary-envelope`
- [ ] `doku/description_volumes_api.md`'s export and populate sections are checked against the shipped endpoints: params, statuses, and error codes all still match; no doku hunk is needed, and if any mismatch is found it is fixed in the same change
- [ ] No code changes in this slice

## Blocked by

- `400-binary-envelope-01-seam-and-volumes` (the conventions text declares a reference that must already be true in the code)
