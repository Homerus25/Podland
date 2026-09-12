---
title: The PruneReportDialog module and its pinned suite, with images as the tracer adapter
parent: 360-prune-report-dialog
date: 2026-09-12
status: open
labels:
  - ready-for-agent
  - frontend
  - shared
  - images
  - testing
author: vibe-agent
---

## Parent

`.scratch/360-prune-report-dialog/issue.md` (read its "Decisions" section first — it settles the design; do not relitigate)

## What to build

The shared PruneReportDialog module lands with exactly the interface the
parent decided, plus the suite that pins its invariants once. The images
feature converts as the tracer adapter: its private prune modal shrinks to
the thin wrapper (same props as today), the module takes over the reset
lifecycle, and the images prune suite thins to what only a feature can
assert. For a user nothing changes — same dialog, same words, same
testids, same behavior — but from this slice on the images prune flow is
rendered by the shared module, and the next prune feature starts at zero.

## Acceptance criteria

- [ ] `frontend/src/shared/components/PruneReportDialog.tsx` exports `PruneReportDialog` generic over an entry constrained to `{ size: number; error?: string }`, with exactly the parent-decided props: the mutation slice (including `data` and `reset`), the singular `resource` noun, `entryKey`, `renderLabel`, and `onClose`
- [ ] Before mutating, the dialog shows the noun-derived title ("Prune unused images"), the two-paragraph confirm body, a ghost Cancel and a red pending-wired "Prune images" confirm button under the `confirm-prune-button` testid, and the default error region via `ErrorDisplay` + `getApiError`
- [ ] On success the dialog stays open: title "Prune report", footer reduced to Close (`close-prune-report-button`), and the report table with "Removed image" / "Reclaimed" headers, mono `renderLabel` cells, a red `entry.error` cell when present and `formatBytes(entry.size)` otherwise, rows keyed by `entryKey` under the `prune-report` / `prune-report-row` testids
- [ ] An empty report renders "Nothing to prune: no unused images on this machine." under the `prune-report-empty` testid
- [ ] `reset()` runs on mount and on unmount; cancel and close send no request
- [ ] The module suite pins all of the above with a stub mutation object and no QueryClient
- [ ] The images list renders its prune dialog through the module via the thin `PruneImagesModal` keeping today's `{ onClose, pruneImages }` prop shape; no manual `reset()` call remains in the images feature (the toolbar button only opens, close only closes)
- [ ] The images prune suite keeps the read-level gating test, the adapter-wiring smoke (opens the confirm, sends nothing), and the prune-on-confirm + list-refetch flow minus report-detail assertions; its per-entry-error and empty-report tests are deleted as module-owned
- [ ] The full frontend suite is green and `npx biome check` passes on the changed files

## Blocked by

- None (can start immediately)

## Technical notes

- Reuse the existing `Modal` shell, `ErrorDisplay`, and `formatBytes`; like MutationDialog, the module never touches react-query — callers hand over the structural slice (plus `data`).
- The dialog has no `isOpen` prop: consumers render it conditionally, so mount is open and unmount is close — the same contract MutationDialog established.
