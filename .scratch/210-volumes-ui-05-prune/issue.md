---
title: Volume prune with report
parent: Volumes UI — list, inspect drawer, create, delete, prune, export, populate
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - frontend
  - feature
author: vibe-agent
---

## Parent

Volumes UI epic (`.scratch/210-volumes-ui/issue.md`); backend contract in `.scratch/200-volumes-management/200-0-volumes-management-master.md`.

## What to build

A Prune toolbar action in the Volumes tab (write-level users only) behind a danger confirmation that spells out the blast radius: all unused volumes on this machine will be removed; in-use volumes are safe. After confirmation, the result renders as a report listing each removed volume with its reclaimed size (and any per-volume errors), and the volumes query is invalidated.

## Acceptance criteria

- [ ] Prune button visible only to write-level users
- [ ] Danger confirmation precedes the call, stating that all unused volumes are removed
- [ ] Result view lists removed volumes with reclaimed sizes, and any per-volume errors
- [ ] Volumes list refreshes after prune
- [ ] MSW tests: confirmation gating, report rendering, invalidation, hidden button for read users

## Blocked by

- 200-volumes-management-05-prune (prune endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
