---
title: ImageList adapter - one triad, toolbar always visible
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - images
  - refactoring
  - testing
author: vibe-agent
---

## Parent

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first, especially the behavior-change ledger (decision 7);
this slice absorbs most of it.

## What to build

ImageList's stacked early-returns delete and the module's inline triad takes
over, which changes what a user sees in three states: during loading, on
error, and when empty, the pull form and prune button now render (today the
whole toolbar vanishes). A read-only user no longer sees the pull form - it is
a write - and the hint shown instead is the canonical copy with
`data-permission="read"` (ImageList today uses its own wording and omits the
attribute). The empty state keeps its "Pull one to get started." sentence via
the emptyMessage override, and that sentence finally tells the truth: the form
sits right above it. The left-aligned pull-form layout is preserved by
passing a `flex={1}` box as the first toolbar child of the module's wrapper.

## Acceptance criteria

- [ ] No early-return variant remains; the toolbar renders in every list
      state (loading, error, empty, table)
- [ ] Read-only users get the canonical images hint (testid
      `images-permission-hint`, `data-permission="read"`) and see neither the
      pull form nor the prune button
- [ ] Empty copy keeps "Pull one to get started." via `emptyMessage`; the
      error branch carries `images-error`
- [ ] Pull form remains left-aligned with the prune button right-aligned,
      visually unchanged for permitted users
- [ ] ImageList's suite deletes its triad-exclusion tests, keeps one loading
      smoke, and keeps delete, force/409-hint, and pull assertions green
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

310-2-containers-adapter

## Technical notes

- The pull form itself is untouched feature code; only where it renders moves
  (into the toolbar slot, inside a `flex={1}` box).
- The prune button keeps its reset-on-open wiring exactly; dialog machinery is
  C6's, not this epic's.
