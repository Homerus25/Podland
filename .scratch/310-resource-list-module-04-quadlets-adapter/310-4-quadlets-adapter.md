---
title: QuadletList adapter - uniform permission gating
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - quadlets
  - refactoring
  - testing
author: vibe-agent
---

## Parent

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first; this slice turns QuadletList's partial gating into
the uniform switch (decisions 1, 6, 7).

## What to build

QuadletList converts and its partial gating becomes uniform. Today only the
two auto-update buttons are gated on the permission check (create is always
visible) and there is no hint at all. After this slice a read-only user gets
the canonical quadlets hint instead of the whole toolbar - create is a write,
so it hides with the rest, a hide-and-hint fix. The `permission` prop becomes
required like its siblings (MachineDetailPage already passes it). The two
trailing action columns become label-less column entries. The auto-update
toolbar buttons, their run-before-modal flow, lifecycle error sub-rows, and
the logs link stay feature code, unchanged.

## Acceptance criteria

- [ ] Read-only users get the canonical quadlets hint (testid
      `quadlets-permission-hint`, `data-permission="read"`) and see neither
      create nor the auto-update buttons
- [ ] The `permission` prop is required; MachineDetailPage needs no change
- [ ] The two trailing action columns are label-less column entries
- [ ] Auto-update's check-updates/update-all flows and lifecycle error
      sub-rows render unchanged
- [ ] QuadletList's suite deletes its triad-exclusion tests, keeps one loading
      smoke, and keeps create, lifecycle, delete, logs, edit, and auto-update
      assertions green
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

310-3-images-adapter

## Technical notes

- QuadletList's toolbar is the busiest of the five (create plus two gated
  auto-update buttons); if the module's toolbar slot needs anything new to
  carry it, fold it into the module here rather than working around it in the
  feature.
