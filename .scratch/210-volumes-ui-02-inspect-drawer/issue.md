---
title: Volume inspect drawer
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

Clicking a volume row opens a drawer showing the volume's full configuration: name, driver, labels (key/value), options, mountpoint, creation timestamp, and anonymous flag — everything the inspect endpoint returns. Handles the stale-row case: a 404 from inspect surfaces a clear message and triggers a list refetch, so deleted volumes vanish instead of erroring forever.

## Acceptance criteria

- [ ] Clicking a row opens a drawer populated from the inspect endpoint
- [ ] Labels and options render as key/value structures; all returned fields are displayed
- [ ] A 404 on inspect shows a clear message and refreshes the volume list
- [ ] Connection/server errors show the standard error treatment
- [ ] MSW tests: drawer opens with data; 404 path triggers refetch; error state renders

## Blocked by

- 200-volumes-management-02-inspect (inspect endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
