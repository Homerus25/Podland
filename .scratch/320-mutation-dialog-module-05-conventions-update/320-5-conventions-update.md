---
title: Conventions update and reset-site ledger
parent: MutationDialog module - one owner for the mutation-dialog machinery thirteen dialogs hand-rolled
date: 2026-09-11
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - docs
  - refactoring
  - testing
author: vibe-agent
---

## Parent

MutationDialog epic (320-0-mutation-dialog-module-master) - read its "Design
decisions" section first; this slice closes the epic by making the module
the documented owner of the dialog machinery.

## What to build

The frontend conventions doc replaces its "Handoff: MutationDialog candidate
(C6)" section with the settled ruling: every mutation dialog renders through
MutationDialog (forms) or ConfirmDialog (destructive confirms); the module
owns the reset lifecycle, pending wiring, close-on-success, and the error
region; callers supply the body, the mutation hook's result, the confirm
variables (confirms), and the success side effect through the seams - toast,
navigate, or silent close is an explicit choice at each call site, never a
copy-paste accident. The doc names the smallest reference adapters to copy
(the secret delete confirm and one create modal) and records the
error-code-to-hint pattern as renderError adapters.

The MachinesPage comment documenting the reset-on-open/close invariant
deletes - the invariant stopped being caller knowledge. A final ledger
confirms the migration is complete: zero `.reset()` call sites remain in the
thirteen dialogs, with the only surviving sites being the sanctioned
non-modal ones (secret reveal, share grant/revoke, machine settings form).
The full frontend suite runs green as the epic's gate.

## Acceptance criteria

- [ ] frontend-conventions.md states the MutationDialog/ConfirmDialog ruling
      in its components section (ownership, seams, reference adapters,
      renderError hint pattern) and the C6 handoff section is gone
- [ ] The conventions doc's test section records the pin-once split: the
      module suite owns the skeleton and invariant tests; feature suites
      keep domain assertions
- [ ] The MachinesPage reset-invariant comment is deleted
- [ ] Ledger: a tree-wide search finds no `.reset()` call sites in the
      thirteen dialogs; surviving sites are only the secret reveal flow,
      the share section, and the machine settings inline form
- [ ] `mutationToPromise` remains in use only by the machine settings
      inline form (or is explicitly noted if that changed for other
      reasons)
- [ ] Full frontend suite green and completes under the two-minute budget;
      lint/typecheck green

## Blocked by

- 320-4-quadlet-edit-and-populate
