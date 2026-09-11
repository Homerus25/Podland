---
title: Confirm dialog adapters - containers, images, quadlets, volumes
parent: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - containers
  - images
  - quadlets
  - volumes
  - refactoring
  - testing
author: vibe-agent
---

## Parent

MutationDialog epic (320-0-mutation-dialog-module-master) - read its "Design
decisions" section first; this slice converts the four list-owned delete
dialogs onto the ConfirmDialog preset and exercises both replacement seams.

## What to build

The container, image, quadlet, and volume delete dialogs become ConfirmDialog
adapters. Each keeps its mutation hook and its domain body; the skeleton
(shell, resets, pending, close-on-success, error region) moves to the module.
User-visible behaviour is unchanged except where the settled ledger says
otherwise: volume delete gains reset-on-open (the recorded gap - a stale
error from a previous attempt no longer bleeds into a fresh dialog), and
volume keeps its success and failure toasts, now expressed through the
onSuccess/onError seams instead of inline handlers.

This slice is where the two replacement seams prove out: image's in-use state
swaps the footer button via renderConfirm (`confirm-delete-button` to
`force-delete-button`) and suppresses the default error display in favour of
its hint via renderError; volume composes the default display plus its
in-use hint and force checkbox via renderError. Quadlet keeps its two option
checkboxes in the body with the confirm handler closing over them.

## Acceptance criteria

- [ ] ContainerList's delete dialog is a ConfirmDialog adapter; behaviour
      unchanged (plain envelope on CONTAINER_RUNNING, modal stays open on
      failure)
- [ ] ImageList's delete dialog swaps the confirm button via renderConfirm
      on IMAGE_USED_BY_CONTAINERS (red solid `confirm-delete-button` by
      default, red outline `force-delete-button` in the in-use state) and
      renders the `image-in-use-warning` hint via renderError with the
      default error display suppressed
- [ ] QuadletList's delete dialog keeps stop-first (default on) and force
      options in its body; confirm sends the matching parameters
- [ ] VolumeList's delete dialog composes ErrorDisplay plus the
      `volume-in-use-warning` hint and force checkbox via renderError; the
      `force` parameter is absent by default and present when checked
- [ ] Volume delete keeps success and failure toasts via the seams; row
      removal on success is unchanged
- [ ] Volume delete and quadlet delete no longer depend on reset-on-close
      only - the module's mount/unmount reset covers open and close
- [ ] None of the four features contains hand-rolled dialog skeleton or
      `.reset()` call sites for these dialogs
- [ ] The four delete suites drop skeleton re-pins and keep domain
      assertions: container DELETE path and 409 envelope, image force
      re-request flow, quadlet option mapping and running-resource error,
      volume force parameter, in-use hint, toasts, and read-user gating
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

- 320-1-module-and-secret-confirm-tracer

## Technical notes

- Image's renderConfirm receives the parsed ApiError through the state
  argument; the swap decision keys off the error code exactly as today.
- Volume's renderError renders ErrorDisplay itself (it stays a public shared
  component) followed by the hint - the seam replaces the region, it does
  not append to it.
- The volume row triggers keep passing the pending item to the same setters;
  only the dialog component changes, so the reset-on-open gap closes without
  touching the setters.
