---
title: Volume delete endpoint (DELETE /api/machines/{id}/volumes/{volumeName})
parent: Volume management for machines (list, inspect, create, delete, prune, export, populate)
date: 2026-08-30
status: ready-for-agent
labels:
  - volumes
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

Volume management for machines (see `.scratch/200-volumes-management/200-0-volumes-management-master.md` for full rulings).

## What to build

`DELETE /api/machines/{id}/volumes/{volumeName}?force=<bool>` returning 204 on success. `force` defaults to false. Without force, podman refuses to remove a volume in use by any container — that refusal must surface as a distinct 409-style client error (clear error code plus podman's message), so the UI can explain what happened. With force=true, podman stops dependent containers first (its own semantics, including stop timeout) and then removes the volume.

Write-level permission. Unknown volume names get the same clean 404 as inspect.

## Acceptance criteria

- [ ] `DELETE` of an unused volume returns 204 and the volume disappears from the list
- [ ] Deleting an in-use volume without force returns 409 with a distinct error code and podman's message
- [ ] Deleting with `force=true` succeeds on an in-use volume (dependent container stopped, volume gone)
- [ ] Unknown volume name returns 404; read-level users get 403; no machine access 403
- [ ] Unit tests: force-flag parsing, in-use error mapping, not-found mapping (mocked seams)
- [ ] Integration test against real podman covering the unused, in-use-without-force, in-use-with-force, and unknown-name cases

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
