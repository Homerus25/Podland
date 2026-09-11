---
title: Quadlet Create endpoint
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

Implement `POST /api/machines/{id}/quadlets` end-to-end. Accepts `{name, resource, content}` and installs the quadlet on the remote machine.

Podman's create primitive is `QuadletInstall`, which takes a **tar body** (not raw content). The service must wrap the request `content` as an in-memory tar (single file named `<name>.<resource-extension>`) and pass it to `ContainerEngine.QuadletInstall`. Use the tar-from-reader pattern from `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:161` (`createTarFromReader`) as a reference for the framing.

Returns `201` with the created quadlet DTO (id=name, state=unknown/stopped, content echoed). Map a name collision to `QUADLET_ALREADY_EXISTS` (409).

`QuadletInstallOptions{ReloadSystemd: true}` should be set so systemd picks up the new unit.

## Acceptance criteria

- [ ] `QuadletCreateRequest` DTO with `name`, `resource`, `content` (validated via `go-playground/validator`)
- [ ] `Service.Create` wraps content as a tar body and calls `QuadletInstall` with `ReloadSystemd: true`
- [ ] `Handler.Create` returns `201` with the created quadlet DTO
- [ ] `QUADLET_ALREADY_EXISTS` (409) on name conflict
- [ ] `VALIDATION_FAILED` (400) on missing/invalid fields
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Service unit test (mock) covering success, conflict, connection-failure
- [ ] Handler unit test covering success, validation failure, conflict

## Blocked by

- 070-01-quadlet-list (reuses the response DTO + `resource` derivation)

## Technical notes

- Podman reference: `QuadletInstall(ctx, []string, entities.QuadletInstallOptions) (*entities.QuadletInstallReport, error)` at `~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:26`
- `QuadletInstallReport{InstalledQuadlets map[string]string, QuadletErrors map[string]error}` — a conflict surfaces as an entry in `QuadletErrors`; map the "already exists" message to `ErrQuadletAlreadyExists`
- Resource → filename extension mapping is the inverse of the list slice's suffix derivation
- Contract: `doku/description_api.md:899-935`
