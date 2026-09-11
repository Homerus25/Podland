---
title: Volume export tar download
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

A per-row Export action (visible to any user with machine access — export is read-level by ruling) that downloads the volume as `<name>.tar`. The download goes through an authenticated fetch (reusing the app's token handling) to a blob and triggers a browser save, since the axios JSON client is not a fit for binary streaming. If a small shared download utility is extracted, it lives in the shared utils area and is unit tested.

Row-level busy state while downloading; failures surface as error toasts with the standard message treatment.

## Acceptance criteria

- [ ] Export action triggers a download that saves as `<volumeName>.tar`
- [ ] Request is authenticated like all other API calls
- [ ] Per-row busy state during download; error toast on failure
- [ ] Action is available to read-level users (all machine-access users)
- [ ] MSW tests: blob request intercepted, filename/save invocation asserted, busy and error states

## Blocked by

- 200-volumes-management-06-export (export endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
