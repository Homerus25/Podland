---
title: Quadlet List endpoint
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

Implement the `GET /api/machines/{id}/quadlets` endpoint end-to-end: service method, handler, response DTO, and tests. This is the foundation slice — it establishes the `entities.ListQuadlet` → response DTO mapping and the `id`/`resource`/`state` derivation rules that every later read slice reuses.

The service calls the podman tunnel `ContainerEngine.QuadletList` (note: the method is on `ContainerEngine`, **not** a `QuadletEngine`) and maps each `entities.ListQuadlet` to the response shape. Per the epic's no-persistence ruling:

- `id` = the quadlet **name** (podman has no uuid)
- `resource` derived from filename suffix (`.container`→container, `.pod`→pod, `.image`→image, `.volume`→volume, `.network`→network, `.kube`/`.build`→ map to the closest contract value, default if unknown)
- `state` parsed from the systemd `Status` string into `unknown|stopped|running|error` (heuristic substring match, e.g. contains "running"→running)
- `path`, `unit` (= `UnitName`), `raw` from the entity

Support the `resource` and `name` query filters (map to podman list filters where possible; otherwise filter in the service after fetching).

## Acceptance criteria

- [ ] `Service.List(ctx, userID, machineID, filters)` calls `ContainerEngine.QuadletList` and returns mapped DTOs
- [ ] `Handler.List` wired and returns `200` with a JSON array of quadlets
- [ ] `id` is the quadlet name; `resource` derived from suffix; `state` parsed from `Status`
- [ ] `resource` and `name` query parameters filter the result
- [ ] `CONNECTION_FAILED` returned on connection errors; auth/permission errors handled consistently with images
- [ ] Service unit test with a mock `ConnectionManagerInterface` asserting the podman call and mapping
- [ ] Handler unit test covering success, empty list, connection failure, and missing machine

## Blocked by

- 070-00-quadlet-service-testability

## Technical notes

- Podman reference: `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:77` `QuadletList(ctx, entities.QuadletListOptions) ([]*entities.ListQuadlet, error)`
- Entity shape: `entities.ListQuadlet{Name, UnitName, Path, Status, App, Pod}` (`~/repos/podman/pkg/domain/entities/quadlet.go:32`)
- Reuse the `concreteConnectionManager` + `tunnel.ImageEngine{ClientCtx: connCtx}` pattern from `internal/domain/images/service.go`, but use `tunnel.ContainerEngine` for quadlet calls
- Contract: `doku/description_api.md:834-865`
