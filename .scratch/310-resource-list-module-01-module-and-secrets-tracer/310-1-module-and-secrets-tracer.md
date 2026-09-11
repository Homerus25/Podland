---
title: ResourceList module + SecretList tracer
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
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

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first; this slice turns those decisions into the module and
proves it on the smallest of the five lists.

## What to build

A ResourceList component lands in shared/ and becomes the single owner of the
machinery every feature list hand-rolls today: the toolbar-or-hint row, the
loading/error/empty/table triad, the table chrome, and the testid derivation.
SecretList converts to it as the reference adapter. For a permitted user the
rendered behaviour is unchanged; for a read-only user the already-correct hint
keeps its meaning under the canonical copy; the one visible change is the
error branch gaining its mandated testid.

The module's own suite pins, once, what all five feature suites pin today:
the triad states are mutually exclusive and each carries its testid; the
toolbar swaps for the hint when the permission check fails; label-less columns
render bare action headers; the empty-copy override works.

## Acceptance criteria

- [ ] ResourceList renders the toolbar (or hint) above an inline triad;
      loading/error/empty/table are mutually exclusive, with testids
      `{resource}s-loading`, `{resource}s-error`, `{resource}s-empty`,
      `{resource}s-table` derived from the plural resource prop
- [ ] The error branch renders ErrorDisplay with getApiError and the
      `{resource}s-error` testid; the empty branch renders the default copy
      `No {resource} on this machine yet.` unless `emptyMessage` overrides it
- [ ] With `canManage: false`, the toolbar is hidden and the hint renders with
      the canonical copy, testid `{resource}s-permission-hint`, and
      `data-permission="read"`; with `canManage` omitted, the list is ungated
      and no hint renders
- [ ] Columns without a label render a bare action-column header; `isNumeric`
      passes through to the header cell
- [ ] SecretList contains no hand-rolled triad, table chrome, or testid
      derivation; its permission-conditional action column is expressed in the
      columns array
- [ ] SecretList's suite deletes its triad-exclusion tests, keeps one loading
      smoke, and keeps its row, detail-reveal, create, and delete assertions
      green
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

None (can start immediately)

## Technical notes

- `renderRow(item)` returns the feature's Row element and must set its own
  React key; the module maps over `items` and adds no wrapper element.
- The module derives only the plural testids; the singular row testid
  (`secret-row`) stays on the feature's Row component.
- Loading copy is module-owned: `Loading {resource}…`.
- The toolbar slot is wrapped in `HStack justify="flex-end" mb={2}` by the
  module; SecretList's create button is the slot's only child.
