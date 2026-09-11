---
title: VolumeList adapter - clickable rows over shared chrome
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - volumes
  - refactoring
  - testing
author: vibe-agent
---

## Parent

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first, especially the C6 boundary (decision 8); this slice
converts the heaviest list and must not touch its dialog machinery.

## What to build

VolumeList, the heaviest of the five, converts last. Its clickable rows (row
click opens the inspect drawer, row buttons stopPropagation) ride the shared
chrome through renderRow; the export, populate, and delete row buttons, the
force-checkbox dialog with its VOLUME_IN_USE warning, and the drawer itself
are untouched feature code - the MutationDialog candidate owns that machinery
later. VolumeList is the only list already carrying the error testid, so this
conversion is purely ownership moving: rendered behaviour is identical for
every user in every state.

## Acceptance criteria

- [ ] VolumeList renders through ResourceList; triad, chrome, testids
      (`volumes-error` included), toolbar, and hint are module-owned
- [ ] Row click still opens the inspect drawer; row action buttons still
      stopPropagation; export, populate, and delete flows are unchanged
- [ ] Delete and populate dialogs are untouched, including their
      reset-on-close (their missing reset-on-open stays as-is; it is
      recorded as C6 input in slice 6)
- [ ] VolumeList's suite deletes its triad-exclusion tests, keeps one loading
      smoke, and keeps drawer, create, delete, force, export, populate, and
      prune assertions green
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

310-4-quadlets-adapter

## Technical notes

- VolumeList is the only list with row-level onClick; if the module's
  renderRow contract needs a documented caveat for it (stopPropagation
  responsibility), document it in the module, not in VolumeList.
