---
title: Quadlet Delete endpoint
parent: 070-quadlets-management
date: 2026-08-02
status: completed
labels:
  - completed
  - quadlets
  - backend
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## What to build

Implement `DELETE /api/machines/{id}/quadlets/{quadletId}` end-to-end. Removes a quadlet via `ContainerEngine.QuadletRemove`, honouring the `stop` and `force` query parameters.

Map the query params to `entities.QuadletRemoveOptions`:

- `force=true` → `Force: true` (remove even if the managed resource is running)
- `stop` (default true) — when the resource is running and `force` is not set, return `QUADLET_RESOURCE_RUNNING` (400); otherwise stop-then-remove
- `ReloadSystemd: true`

Returns `204 No Content` on success. Map a missing quadlet to `QUADLET_NOT_FOUND` (404).

## Acceptance criteria

- [ ] `Service.Delete(ctx, userID, machineID, name, force, stop)` calls `QuadletRemove`
- [ ] `Handler.Delete` parses `quadletId`, `force`, `stop` and returns `204`
- [ ] `QUADLET_RESOURCE_RUNNING` (400) when running and `force` not set
- [ ] `QUADLET_NOT_FOUND` (404) when the quadlet doesn't exist
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Service unit test (mock) covering success, running-without-force, not-found, connection-failure
- [ ] Handler unit test covering success, running-conflict, not-found

## Blocked by

- 070-01-quadlet-list

## Technical notes

- Podman reference: `QuadletRemove(ctx, []string, entities.QuadletRemoveOptions) (*entities.QuadletRemoveReport, error)` at `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:108`
- `QuadletRemoveReport{Removed []string, Errors map[string]error}` — a missing quadlet surfaces in `Errors`; map to `ErrQuadletNotFound`
- "Running" detection may require a List/Status lookup before remove; if podman returns a typed "in use" error on Force=false, map that to `ErrQuadletResourceRunning`
- Contract: `doku/description_api.md:965-984`
