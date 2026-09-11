---
title: Volume populate upload modal
parent: Volumes UI — list, inspect drawer, create, delete, prune, export, populate
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - frontend
  - feature
author: vibe-agent
---

## Parent

Volumes UI epic (`.scratch/210-volumes-ui/issue.md`); backend contract in `.scratch/200-volumes-management/200-0-volumes-management-master.md`.

## What to build

A per-row Populate action (write-level users only) opening an upload modal supporting both backend shapes: pick one or more plain files (their relative paths are preserved via the file parts) or a single tar archive. The modal states clearly that populate **merges** — uploaded paths overwrite same-named files, other contents are kept — so nobody expects a wipe. Submit builds the multipart request, shows upload progress, and on success closes with a toast.

The 413 size-limit response renders with an actionable message (upload exceeds the configured limit — split or shrink it), not a generic error.

## Acceptance criteria

- [ ] Populate action visible only to write-level users
- [ ] Modal accepts multiple files or one tar; submitting both shapes at once is prevented client-side
- [ ] A visible notice explains merge semantics before submit
- [ ] Multipart payload carries file parts with relative paths (or the single archive part) as the backend contract requires
- [ ] Upload progress indicator during submit; success closes modal with toast
- [ ] 413 responses render an actionable size-limit message; other errors use standard treatment
- [ ] MSW tests: FormData payload shape (parts and filenames), merge notice present, 413 handling, hidden action for read users

## Blocked by

- 200-volumes-management-07-populate (populate endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
