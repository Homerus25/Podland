---
title: Volume inspect endpoint (GET /api/machines/{id}/volumes/{volumeName})
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

`GET /api/machines/{id}/volumes/{volumeName}` returning the full configuration of a single volume, resolved by exact name: name, driver, labels, options, mountpoint, creation timestamp, and anonymous flag. Read-level permission, consistent with list.

Unknown volume names must produce a clean 404 with a distinct not-found error code, so stale UI state is diagnosable rather than ambiguous.

## Acceptance criteria

- [ ] `GET /api/machines/{id}/volumes/{volumeName}` returns 200 with the volume's full config
- [ ] Unknown volume name returns 404 with a distinct error code in the standard envelope
- [ ] Users without machine access get 403; unknown machine IDs get 404
- [ ] Unit tests: handler and service tests for the happy path and not-found mapping (mocked service / connection seam)
- [ ] Integration test against real podman: inspect a volume created in the harness, then verify 404 for a nonexistent name

## Blocked by

- 200-volumes-management-01-list (volumes domain skeleton and route family)
