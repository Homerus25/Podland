---
title: Machine delete normalization and the five create forms
parent: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - machines
  - volumes
  - containers
  - quadlets
  - secrets
  - refactoring
  - testing
author: vibe-agent
---

## Parent

MutationDialog epic (320-0-mutation-dialog-module-master) - read its "Design
decisions" section first; this slice lands the one accepted behaviour change
(machine delete) and moves all five create form modals onto the core.

## What to build

Machine delete adopts ConfirmDialog with the settled normalization: the
failure error renders in-modal and the modal stays open on failure (today it
closes on any settle with the error on the page behind the modal); the
navigate-away on success stays, expressed through onSuccess. Its always-
mounted modal switches to conditional mounting so the module's mount/unmount
reset covers it (closing the open-only deviation).

The five create form modals - volume, container, quadlet, secret, and
machine add - ride the MutationDialog core. Each form keeps its react-hook-
form body, its validation and disabled-until-valid logic, and its mutation
hook; it drops its internal error JSX (the core's error region takes over),
its await-bridging (the submit handler calls mutate directly; the module
closes on success), and its reset call sites. Volume create keeps its
success toast via onSuccess. The add-machine modal on the machines page
switches to conditional mounting like its siblings.

For a user, creating any resource behaves exactly as today; deleting a
machine fails visibly inside the modal instead of behind it.

## Acceptance criteria

- [ ] Machine delete is a ConfirmDialog adapter: navigate-away preserved via
      onSuccess, error rendered in-modal, modal stays open on failure, modal
      conditionally mounted
- [ ] The five create form modals are MutationDialog adapters with no
      hand-rolled shell, internal error JSX, await-bridging, or `.reset()`
      call sites; forms call mutate directly from their submit handlers
- [ ] Form-level behaviour is unchanged: disabled-until-valid gating, field
      validation, and create-success closing the modal all hold; volume
      create still toasts on success
- [ ] The add-machine modal and the machine delete modal are conditionally
      mounted; no always-mounted `isOpen` dialogs remain
- [ ] Feature suites keep domain assertions (validation gating, envelope
      errors, success close, volume toast) and drop skeleton re-pins; the
      machine detail suite asserts the normalized failure behaviour
      (in-modal error, modal open)
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

- 320-2-confirm-adapters

## Technical notes

- Machine delete's whole-machines-family invalidation stays in its feature
  hook untouched - the module only receives the mutation result.
- The machine add form's file reading and setError flow is local form state;
  it does not interact with the module seams.
- Quadlet create is one branch of the shared quadlet form component; edit
  mode converts in slice 4, so this slice touches only the create path's
  host modal.
