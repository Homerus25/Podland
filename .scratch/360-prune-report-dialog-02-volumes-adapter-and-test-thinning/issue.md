---
title: The volumes twin dies; both features reach their final thinned shape
parent: 360-prune-report-dialog
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - volumes
  - testing
author: vibe-agent
---

## Parent

`.scratch/360-prune-report-dialog/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The volumes feature converts to the shared PruneReportDialog the way
images did in slice 01: the private `PruneVolumesModal` shrinks to the
thin wrapper (same props as today), the module takes over the reset
lifecycle, and the volumes prune suite thins to gating, the adapter-wiring
smoke, and the domain flow. With this slice the volumes twin — its private
`PruneReportList` and the old modal body — is deleted, and the roughly 90
duplicated lines exist exactly once, inside the module. The module itself
does not change: volumes enters as a second adapter, proving the seam.

## Acceptance criteria

- [ ] The volumes list renders its prune dialog through the module via the thin `PruneVolumesModal` keeping today's `{ onClose, pruneVolumes }` prop shape; no manual `reset()` call remains in the volumes feature
- [ ] Volume entries key and label on `entry.name`; nothing volume-specific enters the module — the feature passes only the noun and the two entry accessors
- [ ] The volumes prune suite keeps the read-level gating test, the adapter-wiring smoke, and the prune-on-confirm + list-refetch flow, mirroring images; its per-entry-error and empty-report tests are deleted
- [ ] Both private `PruneReportList` twins and both old modal bodies are gone; a repo-wide search finds exactly one prune-report implementation
- [ ] The module and its suite are unchanged by this slice
- [ ] The full frontend suite is green and `npx biome check` passes on the changed files

## Blocked by

- `360-prune-report-dialog-01-module-and-images-tracer`
