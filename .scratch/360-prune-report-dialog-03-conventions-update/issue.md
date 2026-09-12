---
title: The frontend conventions record the PruneReportDialog vocabulary
parent: 360-prune-report-dialog
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - docs
author: vibe-agent
---

## Parent

`.scratch/360-prune-report-dialog/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

`docs/frontend-conventions.md` catches up with the landed module so the
next slice starts from knowledge instead of reading siblings. The
mutation-dialog vocabulary gains its third shape: form modals ride
`MutationDialog`, destructive confirms ride its `ConfirmDialog` preset, and
stay-open report dialogs (a mutation whose success payload is rendered
in-place) ride `PruneReportDialog`. The "non-modal mutation sites keep
feature-local resets" list drops the prune sites — their resets are now
module-owned. The tests section's pinned-once sentence grows the module
suite alongside `ResourceList` and `MutationDialog`. No `doku/` change:
no endpoint contract moved.

## Acceptance criteria

- [ ] The dialog-vocabulary bullet names all three shapes, what each owns, and the smallest reference adapter to copy, in the file's established voice
- [ ] The feature-local-reset list no longer names the prune sites
- [ ] The tests section's pinned-once sentence covers the new module suite and the thinned feature suites
- [ ] No stale references to per-feature prune-report implementations remain anywhere in `docs/`
- [ ] `doku/` is untouched

## Blocked by

- `360-prune-report-dialog-01-module-and-images-tracer`
- `360-prune-report-dialog-02-volumes-adapter-and-test-thinning`
