---
title: Volume delete with force confirmation
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

A per-row Delete action (write-level users only) opening a danger confirmation dialog mirroring the images/containers force-delete pattern: the volume name is stated plainly, and an explicit force checkbox explains that forcing stops containers using the volume before deletion. Confirm calls the delete endpoint with the force flag; success closes the dialog, invalidates the volumes query, and toasts.

The 409 in-use error (delete without force on an in-use volume) surfaces with podman's message plus a hint that force is required — not a generic failure.

## Acceptance criteria

- [ ] Delete action visible only to write-level users
- [ ] Confirmation dialog shows the volume name and a force checkbox with explanatory text
- [ ] Confirm without force calls delete with force=false; with checkbox, force=true
- [ ] 409 in-use responses render the podman message plus a "force required" hint
- [ ] Volumes list refreshes after successful deletion; success/failure toasts render
- [ ] MSW tests: payload flag, 409 handling, invalidation, hidden action for read users

## Blocked by

- 200-volumes-management-04-delete (delete endpoint)
- 210-volumes-ui-01-list (Volumes tab and list)
