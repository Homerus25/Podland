---
title: ResourceList module - one owner for the list machinery five features hand-rolled
parent: none
date: 2026-09-10
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

Five feature list modules (volumes, quadlets, images, containers, secrets)
hand-roll the same machinery: the loading/error/empty triad, the toolbar with
its reset-on-open buttons, the table chrome, the testid derivation, and the
read-only permission hint. 2,085 lines across five copies, and the copies have
drifted: ImageList implements the settled triad convention differently (stacked
early-returns instead of the inline ternary), and the permission hint exists in
four divergent variants (a fifth lives in SettingsTab). After this epic there
is one deep ResourceList module in shared/ owning the triad, the chrome, the
testids, and the hint copy; each feature keeps only what is genuinely its own
(columns, row rendering, actions). The three-testid convention and the
hide-and-hint pattern are preserved exactly - only ownership moves.

Origin: architecture review 2026-09-09 candidate C5, refined in a grilling
session on 2026-09-10. The decisions below are settled; slices implement them,
they do not reopen them.

## Design decisions (settled in grilling)

1. **All five lists adopt.** Permission gating is an optional prop; a list
   without it (ContainerList today) renders ungated with no hint.
2. **Props-based interface; the module never touches react-query.** Features
   keep their query and mutation hooks and hand the module resolved state:
   `resource` (plural noun), `isPending`, `error`, `items`, `columns`,
   `renderRow`, and the optional `toolbar`, `canManage`, `emptyMessage`. The
   module derives the plural testids (`{resource}s-loading`, `-error`,
   `-empty`, `-table`, `-permission-hint`); the singular row testid stays on
   the feature's Row component, because rows are feature-rendered.
3. **Columns are data, not JSX**: `{ label?, isNumeric? }[]`. An entry without
   a label renders the bare action-column header cell. Features control
   action-column count (and SecretList's permission-conditional one) by how
   they build the array.
4. **The triad is inline and the toolbar is always rendered above it.** Loading
   joins the error/empty/table ternary (it stops being an early return), so
   toolbar and hint stay reachable in every state. This is the 4/5 majority
   shape; ImageList's stacked early-returns delete.
5. **The module owns the toolbar wrapper** (`HStack justify="flex-end"
   mb={2}`) and swaps the whole toolbar for the hint when `canManage` is
   false. ImageList reproduces its form-left layout by passing a `flex={1}`
   box as the first toolbar child.
6. **One canonical hint**: "You have read-only access to this machine. Ask an
   owner or admin for write access to manage its {resource}." with testid
   `{resource}s-permission-hint` and `data-permission="read"`. QuadletList,
   which has no hint today, gains it. SettingsTab's two hints are out of scope
   (not a resource list).
7. **Behavior-change ledger accepted.** Three normalizations land as
   drift-fixes: (a) read-only now hides ImageList's pull form and QuadletList's
   create button (both are writes; today they violate the hide-and-hint
   ruling); (b) the toolbar is visible during loading/error/empty; (c) the
   error branch carries `{resource}s-error` on every list (the conventions doc
   already mandates it; only VolumeList complies today).
8. **Strict read-side boundary with the MutationDialog candidate (C6).**
   Confirm dialogs, create modals, reset-on-open/close invariants, and
   error-code-to-hint logic stay feature code, untouched by this epic.
   VolumeList's row-triggered delete/populate dialogs skip reset-on-open
   today; that gap is recorded in slice 6 as a C6 input, not fixed here.
9. **Scaffolding stays out of scope.** queryKeys, the resourceKeys map, the
   api client, hooks, and MachineDetailPage tab wiring are untouched.
   (QuadletList's `permission` prop becomes required like its siblings; the
   page already passes it.)
10. **Tests: pin once, smoke per feature.** The module suite owns triad mutual
    exclusion, testid derivation, the hint swap, and the empty-copy override.
    Feature suites delete their triad-exclusion tests, keep one loading smoke
    (adapter wiring) plus all row/action/domain assertions.
11. **Migration is expand-then-migrate.** The module lands beside the five
    lists (expand); each slice converts one feature with its suites kept green
    (migrate); there is no contract slice because the hand-rolled stripes
    vanish with each conversion.

## Vertical Slices

### 1. ResourceList module + SecretList tracer
**Issue**: 310-1-module-and-secrets-tracer
- The module lands in shared/ with its own suite; SecretList, the smallest
  list, converts as the proof adapter

**Status**: ready-for-agent
**Blocked by**: None (can start immediately)

### 2. ContainerList adapter (ungated)
**Issue**: 310-2-containers-adapter
- Proves the ungated shape: no permission prop, toolbar always visible, no
  hint state

**Status**: ready-for-agent
**Blocked by**: 310-1 (shared module; slices land in numbered order)

### 3. ImageList adapter
**Issue**: 310-3-images-adapter
- Early-return variant deletes; toolbar visible in every state; pull form
  hidden when read-only; empty copy keeps its pull sentence

**Status**: ready-for-agent
**Blocked by**: 310-2

### 4. QuadletList adapter
**Issue**: 310-4-quadlets-adapter
- Partial gating becomes uniform: hint added, create gated, permission prop
  required

**Status**: ready-for-agent
**Blocked by**: 310-3

### 5. VolumeList adapter
**Issue**: 310-5-volumes-adapter
- Clickable rows and the inspect drawer ride the shared chrome; dialog
  machinery untouched

**Status**: ready-for-agent
**Blocked by**: 310-4

### 6. Conventions handover
**Issue**: 310-6-conventions-update
- frontend-conventions.md names ResourceList the list reference; triad testing
  ruling updates; C6 handoff note recorded

**Status**: ready-for-agent
**Blocked by**: 310-5

Slices 2-5 are logically independent (different feature folders) but all
evolve the shared module; the chain keeps module changes single-threaded in
ascending complexity order.

## Handoff to C6 (recorded by slice 6)

- VolumeList's row-triggered Delete/Populate dialogs call their state setters
  directly without `mutation.reset()` first (they only reset on close). The
  MutationDialog module must own reset-on-open so this class of gap disappears.
