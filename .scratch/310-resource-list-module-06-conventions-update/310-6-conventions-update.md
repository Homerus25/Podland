---
title: Conventions handover - ResourceList becomes the list reference
parent: resource list module - one owner for the list machinery five features hand-rolled
date: 2026-09-10
status: ready-for-agent
labels:
  - ready-for-agent
  - frontend
  - shared
  - refactoring
  - documentation
author: vibe-agent
---

## Parent

ResourceList epic (310-0-resource-list-module-master) - read its "Design
decisions" section first; this slice closes the epic by handing the list
conventions to the module that now owns them.

## What to build

The frontend conventions doc catches up with the module. The list reference
shape becomes ResourceList: a new resource tab copies a feature adapter
(SecretList's is the smallest reference), not 600 lines of hand-rolled
machinery. The triad testing ruling updates - the module's suite pins mutual
exclusion and testids once; a feature suite keeps one loading smoke plus
row/action assertions. The permission-hint ruling points at the module's
canonical copy instead of describing per-feature hint rendering. Finally, a
handoff note records the VolumeList reset-on-open gap as input for the
MutationDialog candidate (C6) so that epic starts from it.

## Acceptance criteria

- [ ] frontend-conventions.md's list sections reference ResourceList as the
      owner of triad, chrome, testid derivation, and hint; the adapter shape
      is named as the reference for a new resource tab
- [ ] The triad/hint testing ruling reads: module suite pins exclusion and
      testids; feature suites keep one loading smoke plus row/action
      assertions
- [ ] The VolumeList reset-on-open gap is recorded where the MutationDialog
      epic will find it (per the master's "Handoff to C6" section)
- [ ] Deletion test satisfied: no feature list contains a hand-rolled triad,
      table chrome, or testid derivation; no early-return loading variant
      remains anywhere
- [ ] Frontend checks green (tests, lint/typecheck)

## Blocked by

310-5-volumes-adapter
