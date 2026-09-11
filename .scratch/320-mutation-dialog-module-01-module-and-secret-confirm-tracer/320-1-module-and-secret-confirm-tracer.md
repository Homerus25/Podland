---
title: MutationDialog + ConfirmDialog modules and the SecretList delete tracer
parent: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - shared
  - secrets
  - refactoring
  - testing
author: vibe-agent
---

## Parent

MutationDialog epic (320-0-mutation-dialog-module-master) - read its "Design
decisions" section first; this slice turns those decisions into the module
pair and proves the confirm shape on the smallest of the six delete dialogs.

## What to build

A MutationDialog core and a ConfirmDialog preset land together in one shared
components file and become the single owner of the machinery every mutation
dialog hand-rolls today: the modal shell, the reset-on-open/reset-on-close
lifecycle, the pending wiring, close-on-success, and the in-modal error
region. The core never touches react-query - it receives the caller's
mutation result and drives it. ConfirmDialog adds the danger footer: ghost
cancel, red confirm carrying the `confirm-delete-button` testid, isLoading
wired to the mutation's pending state.

SecretList's delete dialog converts as the reference adapter, keeping its
per-row mutation instance and its plain-envelope error handling (no
code-specific hint - SECRET_IN_USE stays a comment-level convention). For a
user, deleting a secret behaves exactly as today, including staying open with
the envelope on 409/404 - only ownership moves.

The module's own suite pins, once, what all feature suites pin today, tested
through a stub mutation object with no QueryClient: the reset lifecycle,
cancel-sends-nothing, pending, the default error region, close-on-success
with the side-effect seams fired exactly once, and both replacement seams.

## Acceptance criteria

- [ ] MutationDialog receives a mutation result (`{ mutate, isPending, error,
      isSuccess, reset }`), a title, an onClose, and a body; it resets the
      mutation on mount and on unmount and never calls react-query itself
- [ ] On mutation success the dialog calls the caller's `onSuccess` seam
      exactly once (ref-guarded), then closes via onClose; on failure it
      calls `onError` and stays open
- [ ] The core renders an error region above the body: by default
      ErrorDisplay with getApiError; `renderError(apiError)` replaces the
      whole region when provided
- [ ] ConfirmDialog renders the danger footer - ghost cancel wired to
      onClose, red confirm with `data-testid="confirm-delete-button"` and
      isLoading from the mutation - and obtains the mutation variables from
      the caller's confirm handler
- [ ] `renderConfirm({ error, isPending, onConfirm })` replaces the default
      confirm button when provided
- [ ] The module suite uses a stub mutation object (no QueryClient) and pins
      reset-on-mount, reset-on-unmount, cancel-sends-nothing, pending
      disablement, default error region, close-on-success ordering and
      single-fire, and both replacement seams
- [ ] SecretList's delete dialog is a ConfirmDialog adapter with no
      hand-rolled shell, reset calls, or error JSX; its per-row mutation
      instance is unchanged
- [ ] SecretList's delete suite drops its skeleton re-pins (cancel,
      pending, generic in-modal error) and keeps its domain assertions:
      DELETE by name on confirm, 409/404 keep the modal open with the
      envelope, read users get no delete button, success closes and
      refetches
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

None (can start immediately)

## Technical notes

- Consumers render the dialog conditionally (mount = open); the module
  passes an always-open shell, matching how Modal consumers mount today.
- The stub mutation object in the module suite is a plain object with
  jest/vi spy functions - the interface is the test surface.
- ConfirmDialog's confirm handler returns the variables object; option
  state (checkboxes) lives in the caller's body, and the handler closes over
  it.
