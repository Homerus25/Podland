---
title: Volume list endpoint (GET /api/machines/{id}/volumes)
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

The first slice of the volumes backend: a new `volumes` domain package following the containers pattern (handler → service → podman bindings over the cached per-machine SSH connection). Implement `GET /api/machines/{id}/volumes` returning every volume on the machine, named and anonymous alike. Each list item carries: name, driver, creation timestamp, anonymous flag, labels, options, mountpoint, and size/usage when podman reports it.

The route lives inside the existing machine-access subtree in the app wiring at read level, so any user with a machine-access row can call it. This slice also establishes the reusable skeleton every later volumes slice plugs into: the service interface seam (with a mock in the shared test utilities), DTOs, route registration, and the API contract section in the docs directory for the volumes family.

## Acceptance criteria

- [ ] `GET /api/machines/{id}/volumes` returns 200 with a JSON array of volumes
- [ ] Each volume item includes name, driver, createdAt, anonymous, labels, options, mountpoint, and size when podman reports it
- [ ] Anonymous volumes are included alongside named ones
- [ ] Users without machine access get 403; unknown machine IDs get 404 (existing middleware behavior, verified by test)
- [ ] Connection failures surface as 5xx via the standard error envelope with requestId
- [ ] Unit tests: handler tests with a mocked volume service (mock added to shared test utilities); service tests for validation and connection-failure paths
- [ ] Integration test against real podman: a volume created via the bindings in the harness appears in the list response
- [ ] API contract for the volumes family added to the docs directory

## Blocked by

None - can start immediately
