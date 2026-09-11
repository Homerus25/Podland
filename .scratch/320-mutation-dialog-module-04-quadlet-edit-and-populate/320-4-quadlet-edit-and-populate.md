---
title: Quadlet edit and volume populate on the core
parent: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - quadlets
  - volumes
  - refactoring
  - testing
author: vibe-agent
---

## Parent

MutationDialog epic (320-0-mutation-dialog-module-master) - read its "Design
decisions" section first; this slice converts the two modals that do not fit
the plain confirm or plain form shapes.

## What to build

The quadlet edit modal and the volume populate modal become MutationDialog
adapters, proving the core on the two odd shapes.

Quadlet edit keeps its in-modal entity loading in the body (the query's
loading and error states are the body's own - the core has no query
knowledge, by ruling) and keeps its always-refetch hook; the mutation side -
error region, close-on-success, and the close-only reset deviation - moves
to the core. Editing a quadlet behaves as today.

Populate keeps its local-state body (files input or tar file, the shape-
conflict guard) and its custom footer: the `confirm-populate-button` with
its progress display while pending, expressed through the core's footer
mechanism (the renderConfirm seam exists for exactly this custom-button
shape). Its too-large error maps through renderError to the existing hint;
its success toast stays via onSuccess. The close-only reset deviation
disappears with the module lifecycle.

## Acceptance criteria

- [ ] Quadlet edit is a MutationDialog adapter; its query loading and query
      error states render inside the body, and its mutation error region,
      close-on-success, and reset handling come from the core
- [ ] Quadlet edit keeps its always-refetch behaviour (stale detail is
      never served) and its read-only name/resource fields
- [ ] Populate is a MutationDialog adapter whose footer renders the
      `confirm-populate-button` with progress text while pending; the file
      selection body, shape-conflict guard, and payload building are
      unchanged
- [ ] Populate maps the too-large error code to its existing hint via
      renderError and keeps its success toast via onSuccess
- [ ] Neither dialog contains hand-rolled skeleton or `.reset()` call
      sites; both are covered by the module's mount/unmount reset
- [ ] Both feature suites keep domain assertions (edit content save and
      envelope errors; populate payload shape, too-large hint, progress,
      toast) and drop skeleton re-pins
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

- 320-3-machine-delete-and-create-forms

## Technical notes

- The populate payload builder stays a pure function with its own unit
  tests; the multipart MSW limitation ruling is unaffected.
- Populate's renderError composes the default ErrorDisplay for non-mapped
  codes and the hint for the too-large code - the seam replaces the whole
  region, so the adapter renders both branches itself.
