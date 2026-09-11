---
title: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
parent: none
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - epic
  - frontend
  - shared
  - refactoring
  - testing
author: vibe-agent
---

## What to build

Thirteen dialogs - six confirm-destructive modals (volume, quadlet, container,
image, secret, machine deletes) and seven form modals (create volume, create
container, create quadlet, edit quadlet, create secret, add machine, populate
volume) - hand-roll the same skeleton: modal shell, ghost cancel, red confirm
with pending wiring, in-modal error rendering, close-on-success, and the
reset-on-open/reset-on-close invariant. The invariant is duplicated at 32
`.reset()` call sites with three deviations from itself (volume delete,
volume populate, and quadlet edit reset on close only; machine delete resets
on open only), and success behaviour has drifted three ways (volume toasts,
machine navigates, the rest close silently). Error-code-to-hint wiring
(VOLUME_IN_USE, IMAGE_USED_BY_CONTAINERS, VOLUME_UPLOAD_TOO_LARGE) is
hand-wired per dialog.

After this epic one deep MutationDialog core owns the skeleton - shell, reset
lifecycle, close-on-success, error region, pending - and a thin ConfirmDialog
preset owns the danger footer. Each dialog keeps only what is genuinely its
own: its body, its options, its success side effect expressed explicitly
through a seam. The reset invariant gets one home; the reset-on-open gap
class recorded by the ResourceList epic's conventions handoff disappears.

Origin: architecture review 2026-09-09 candidate C6, refined in a grilling
session on 2026-09-11. The decisions below are settled; slices implement
them, they do not reopen them.

## Design decisions (settled in grilling)

1. **Layered module, all thirteen dialogs adopt.** A `MutationDialog` core
   (shell + reset lifecycle + close-on-success + error region) is consumed
   directly by the form modals; a `ConfirmDialog` preset (ghost cancel, red
   confirm button with the `confirm-delete-button` testid, isLoading wiring)
   serves the six confirms. Both land in one file in shared components,
   mirroring ResourceList's placement.
2. **Props-based interface; the module never touches react-query.** Features
   keep their mutation hooks (and their invalidation choices, including
   machine delete's whole-machines-family invalidation and SecretList's
   per-row mutation instance) and hand the module the mutation result itself -
   the structural slice `{ mutate, isPending, error, isSuccess, reset }`. The
   module suite tests through a stub mutation object with no QueryClient.
3. **The module owns the reset invariant.** `reset()` runs on mount and on
   unmount; consumers render the dialog conditionally, so mount is open and
   unmount is close. One home replaces 32 sites; the close-only deviations
   (volume delete, volume populate, quadlet edit) and machine delete's
   open-only deviation all normalize to the module's lifecycle. The
   MachinesPage comment documenting the invariant deletes because the
   invariant stops being caller knowledge.
4. **The module owns close-on-success**, detected by watching the mutation's
   success state (ref-guarded against double-fire), then invoking the
   caller's `onSuccess` seam and closing. `onError` is the matching seam for
   failure side effects. Forms stop awaiting their mutations; a form's submit
   handler just calls `mutate`.
5. **Behaviour-preserving, with one accepted normalization.** Volume delete
   keeps its success and failure toasts and volume populate keeps its toast
   (all via seams); machine delete keeps its navigate-away (via `onSuccess`).
   The one deliberate change: machine delete's error renders in-modal and the
   modal stays open on failure - today the modal closes on any settle with
   the error rendered on the page behind it.
6. **One error region with one replacement seam.** The core renders
   `ErrorDisplay` with `getApiError(error)` above the body by default; a
   `renderError` prop replaces the whole region. Volume composes display plus
   in-use hint inside its `renderError`; image returns only the in-use hint
   (suppressing the default display); populate maps its too-large code.
   Forms delete their internal error JSX.
7. **Image's confirm-button swap is preserved, not redesigned.** A
   `renderConfirm` seam on ConfirmDialog (receiving error, pending, and the
   confirm handler) lets image keep its `confirm-delete-button` to
   `force-delete-button` swap on the in-use error. The default renders the
   standard red confirm. Unifying image onto volume's checkbox pattern is a
   possible later product change; this epic does not make it.
8. **Non-modal mutation sites stay out of scope.** The secret detail reveal
   flow, the share grant/revoke section, the machine settings inline form,
   and the pull-image toolbar keep their feature-local resets; they share no
   dialog skeleton with the thirteen.
9. **`mutationToPromise` survives.** The machine settings inline form still
   awaits its mutation; the utility is not deleted, only the modal forms stop
   using it.
10. **Tests: pin once, keep domain per feature.** The module suite pins the
    skeleton and the invariant: reset-on-mount, reset-on-unmount,
    cancel-sends-nothing, pending disables confirm, default error region,
    close-on-success plus the onSuccess/onError seams (fired exactly once),
    and both replacement seams. Feature suites drop their skeleton re-pins
    (cancel, pending, generic in-modal error) and keep domain assertions:
    force/stop parameters, error-code hints and envelopes, row removal,
    toasts, navigate, read-user gating.
11. **Migration is tracer-first then adapters, chained** (not parallel):
    each slice evolves the shared module, and the chain keeps module changes
    single-threaded in ascending complexity - the same rhythm the
    ResourceList epic used.

## Vertical Slices

### 1. MutationDialog + ConfirmDialog modules and the SecretList delete tracer
**Issue**: 320-1-module-and-secret-confirm-tracer
- The core and preset land in shared components with the module suite;
  SecretList's delete dialog, the smallest confirm, converts as the proof
  adapter

**Status**: ready-for-agent
**Blocked by**: None (can start immediately)

### 2. Confirm dialog adapters: containers, images, quadlets, volumes
**Issue**: 320-2-confirm-adapters
- The remaining four list-owned confirms convert; image exercises the
  renderConfirm seam, volume exercises renderError plus the toast seams and
  gains reset-on-open (the recorded gap)

**Status**: ready-for-agent
**Blocked by**: 320-1

### 3. Machine delete normalization and the five create forms
**Issue**: 320-3-machine-delete-and-create-forms
- Machine delete adopts the module defaults (in-modal error, stays open on
  failure, navigate via onSuccess); the five create form modals ride the
  core, dropping their await-bridging, internal error JSX, and reset sites

**Status**: ready-for-agent
**Blocked by**: 320-2

### 4. Quadlet edit and volume populate on the core
**Issue**: 320-4-quadlet-edit-and-populate
- The two odd modals convert: edit-quadlet keeps its in-modal query loading
  in the body; populate brings a custom footer button, progress, and the
  too-large hint

**Status**: ready-for-agent
**Blocked by**: 320-3

### 5. Conventions update and reset-site ledger
**Issue**: 320-5-conventions-update
- frontend-conventions.md replaces the C6 handoff with the MutationDialog
  ruling; the MachinesPage invariant comment deletes; a ledger confirms zero
  reset sites remain in the thirteen dialogs

**Status**: ready-for-agent
**Blocked by**: 320-4

Slices 2-4 are logically independent (different feature folders) but all
evolve the shared module; the chain keeps module changes single-threaded in
ascending complexity order.
