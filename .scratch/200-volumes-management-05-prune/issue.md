---
title: Volume prune endpoint (POST /api/machines/{id}/volumes/prune)
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

`POST /api/machines/{id}/volumes/prune` removing all unused volumes on the machine in one call and returning 200 with podman's prune reports: one entry per removed volume with its name and reclaimed size (and any per-volume error podman reports). Volumes in use by containers are untouched.

Write-level permission — this is the mitigation for the multi-user blast radius accepted during grilling.

Routing pitfall: the literal `/prune` route must take precedence over the `/{volumeName}` subtree in the router, or prune calls get swallowed as a volume named "prune".

## Acceptance criteria

- [ ] `POST /api/machines/{id}/volumes/prune` returns 200 with a report entry per removed volume (name + reclaimed size)
- [ ] Unused volumes are gone from the list afterwards; in-use volumes survive
- [ ] Read-level users get 403
- [ ] Unit tests for report mapping and error handling; integration test: seed unused volumes plus one in-use volume, prune, verify reports and survival via the list endpoint

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
