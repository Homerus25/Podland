---
title: One PruneReport dialog (architecture review C4)
parent: none
date: 2026-09-12
status: open
labels:
  - needs-triage
  - frontend
  - shared
  - refactoring
author: vibe-agent
---

## Problem Statement

The prune-report presentation exists twice, once per feature: images
(`frontend/src/features/images/components/ImageList.tsx`) and volumes
(`frontend/src/features/volumes/components/VolumeList.tsx`) each carry a
private `PruneReportList` (33 lines) and a private `Prune*Modal` (57 lines)
that are line-for-line identical apart from three things: the resource noun,
the report entry's key field (`id` shortened through `shortImageId` vs
`name` verbatim), and the report DTO type (`{ id, size, error? }` vs
`{ name, size, error? }`). About 90 duplicated lines own the same machinery:
the footer swap from confirm to report, the empty-report copy, per-entry
error rows, the in-modal error region, and a hand-rolled reset ritual (four
manual `reset()` calls across the two features) that the MutationDialog
module already proved can be module-owned. The deletion test passes: delete
either copy and the identical complexity reappears in the sibling.

The shared MutationDialog cannot absorb this shape: it closes on success,
but a prune dialog's whole point is to stay open and render the report the
mutation returned — and its structural slice carries no `data`.

## Solution

One deep `PruneReportDialog` module in `shared/components/`, with both
features reduced to thin adapters (~10 lines each) handing over their
mutation result, resource noun, and entry accessors. The module owns
everything the twins duplicated: the reset lifecycle, the footer swap, the
report table, the empty-report and confirm copy (noun-derived), and the
error region. No endpoint contract changes, so `doku/` is untouched.

## Decisions (from the design session — do not relitigate)

1. **Build now, don't wait for a third prune feature.** The glossary's
   two-adapters rule already holds (both call sites are live), the deletion
   test passes, and the backend exposes only images and volumes prune
   endpoints — no third prune feature exists or is planned, so deferral
   buys nothing.
2. **Name and home:** `frontend/src/shared/components/PruneReportDialog.tsx`,
   exported as `PruneReportDialog`. Flat in `shared/components/` (no
   subfolder); "Dialog" is the shared-module word, "Modal" the
   feature-instance word.
3. **Interface** — the seam both features cross (from the design session;
   a type shape encodes this more precisely than prose):

   ```tsx
   <PruneReportDialog<TEntry>
     mutation={pruneImages} // { mutate, isPending, error, data: TEntry[] | undefined, reset }
     resource="image"       // singular noun; the plural is derived
     entryKey={(e) => e.id}
     renderLabel={(e) => shortImageId(e.id)}
     onClose={...}
   />
   ```

   `TEntry` is constrained to `{ size: number; error?: string }`. The module
   absorbs reset-on-mount/unmount (callers render it conditionally, so
   mount is open and unmount is close), the confirm/report footer swap with
   a pending-wired confirm button, the report table (mono label cell, red
   error cell vs `formatBytes`, `Removed {resource}` / `Reclaimed`
   headers), the empty-report copy, the confirm body copy, and the default
   error region (`ErrorDisplay` + `getApiError`). No `onSuccess`/`onError`
   seams: invalidation already lives in `useListInvalidatingMutation` and
   no prune site toasts today — add a seam only when a caller needs one.
4. **Copy is module-owned**, derived from `resource`: the titles
   ("Prune unused {plural}" / "Prune report"), the two-paragraph confirm
   body, "Nothing to prune: no unused {plural} on this machine.", and the
   "Prune {plural}" confirm button. Toolbar button labels ("Delete unused
   images" / "Prune") stay feature-owned — they live outside the dialog. A
   `body` escape hatch is added only when a real third feature needs
   different words.
5. **Standalone from MutationDialog.** The two disagree on the headline
   invariant (stays open on success vs closes on success), and
   MutationDialog's slice deliberately carries no `data`. MutationDialog
   and its vocabulary are untouched; revisit a shared core only if a third
   stay-open mutation shape appears.
6. **Testids unchanged:** `confirm-prune-button`, `prune-report`,
   `prune-report-row`, `prune-report-empty`,
   `close-prune-report-button`. They already name the module's parts, not
   the features', so the migration causes no testid churn.
7. **Adapter shape:** the private `PruneImagesModal`/`PruneVolumesModal`
   wrappers survive with today's prop shape (onClose plus the mutation
   result); the mutation hook stays in the list body, mirroring the
   sibling delete modals in the same files. All four manual `reset()`
   calls die — the toolbar button just opens, close just closes.
8. **Test split (pinned once):** a module suite
   (`PruneReportDialog.test.tsx`, stub mutation object, no QueryClient —
   the MutationDialog suite pattern) pins reset on mount/unmount, the
   footer swap, cancel-sends-nothing, stays-open-on-success, report table
   rendering, empty-report copy, noun-derived copy, the error region, and
   the testids. Feature suites keep read-level gating, one adapter-wiring
   smoke (opens the confirm, sends nothing), and the domain flow (request
   shape + list refetch through MSW); the per-entry-error and
   empty-report feature tests are deleted alongside their module-suite
   equivalents.

## Slices

- `360-prune-report-dialog-01-module-and-images-tracer` — the module and
  its pinned suite, with the images feature converted as the tracer
  adapter.
- `360-prune-report-dialog-02-volumes-adapter-and-test-thinning` — the
  volumes twin dies; both feature suites reach their final thinned shape.
- `360-prune-report-dialog-03-conventions-update` — the frontend
  conventions record the module and its vocabulary.
