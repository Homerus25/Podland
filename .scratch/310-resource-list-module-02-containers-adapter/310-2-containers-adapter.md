---
title: ContainerList adapter (ungated)
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - containers
  - refactoring
  - testing
author: vibe-agent
---

## Parent

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first. Slice 1 landed the module; this slice proves the
ungated shape on the list that has no permission concept.

## What to build

ContainerList renders through ResourceList with no permission involvement at
all: every user sees the toolbar (Create container) in every list state, the
triad and table chrome come from the module, and no hint state exists for this
resource. The two trailing action columns become two label-less column
entries. Row lifecycle buttons, their error sub-rows, and the logs link stay
feature-rendered, unchanged.

## Acceptance criteria

- [ ] ContainerList renders through ResourceList with no `canManage` and no
      permission prop: toolbar always visible, no hint state exists
- [ ] The two trailing action columns are label-less column entries
- [ ] ContainerRow's start/stop error sub-rows render unchanged below rows;
      the logs link is untouched
- [ ] ContainerList's suite deletes its triad-exclusion tests, keeps one
      loading smoke, and keeps row, start/stop, delete, create, and logs
      assertions green
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

310-1-module-and-secrets-tracer

## Technical notes

- This is the adapter a future read-only-by-nature resource tab would copy:
  the ungated reference.
