---
title: Volumes tab with volume list
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

Add a **Volumes** tab to the machine detail page (after Containers) rendering the machine's volumes in a table: name, type badge distinguishing named vs anonymous volumes, driver, creation date, and size when the API reports it. Row click is reserved for the later inspect-drawer slice; this slice is read-only display.

Build the feature folder skeleton everything else plugs into: the volumes API client module (list call through the shared axios instance with standard error handling) and a `useVolumes` React Query hook keyed by machine. Distinct loading, error, and empty states. Visible to any user with machine access; no write-gating needed here.

## Acceptance criteria

- [ ] Machine detail page shows a Volumes tab between Containers and Images/Quadlets placement per epic ruling (after Containers)
- [ ] Table renders name, named/anonymous badge, driver, created date, and size when present, from the list endpoint
- [ ] Loading, error, and empty states are distinct and tested
- [ ] API client and hook follow the existing feature-folder conventions (services/, hooks/)
- [ ] MSW tests: hook returns and caches data; component renders rows, badges, and all three states
- [ ] Type definitions for volume DTOs added to `features/volumes/services/volumesApi.ts`, colocated with the list call (per CONTEXT.md frontend DTO-type convention)

## Blocked by

- 200-volumes-management-01-list (list endpoint)
