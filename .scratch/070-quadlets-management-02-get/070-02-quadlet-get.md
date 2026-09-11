---
title: Quadlet Get endpoint
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

Implement `GET /api/machines/{id}/quadlets/{quadletId}` end-to-end. The `quadletId` path parameter is the quadlet **name** (per the epic's `id`=name ruling).

The service composes two podman calls:

- `ContainerEngine.QuadletPrint(name)` for the raw file **content**
- `ContainerEngine.QuadletList` (filtered to this name) for the **metadata** (path, unit, status → state, resource from suffix)

Returns a single-item DTO including `content`. Per the no-persistence ruling, `generation`/`createdAt`/`updatedAt` are **omitted** from the response (documented deviation from the contract).

If the quadlet does not exist (Print/List return not-found), return `QUADLET_NOT_FOUND` (404).

## Acceptance criteria

- [ ] `Service.Get(ctx, userID, machineID, name)` returns content + metadata DTO
- [ ] `Handler.Get` parses `quadletId` path param and returns `200` with the quadlet (including `content`)
- [ ] `QUADLET_NOT_FOUND` (404) returned when the quadlet doesn't exist
- [ ] `generation`/`createdAt`/`updatedAt` omitted (deviation documented in code)
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Service unit test (mock) covering found / not-found / connection-failure
- [ ] Handler unit test covering success, not-found, missing machine

## Blocked by

- 070-01-quadlet-list (reuses the list DTO + `resource`/`state` derivation)

## Technical notes

- Podman reference: `QuadletPrint(ctx, name string) (string, error)` at `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:104`
- "Not found" detection: podman's Print/List do not return a typed not-found error — detect via empty content + empty list match, and map to `ErrQuadletNotFound`
- Contract: `doku/description_api.md:868-895`
