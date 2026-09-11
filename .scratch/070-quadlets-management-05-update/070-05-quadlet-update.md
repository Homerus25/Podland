---
title: Quadlet Update endpoint
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

Implement `PUT /api/machines/{id}/quadlets/{quadletId}` end-to-end. Replaces an existing quadlet's content. Podman has no dedicated update primitive, so the service composes `QuadletRemove` + `QuadletInstall` (with `Replace: true`) of the new content.

Accepts `{content}` (the updated quadlet file content). Flow:

1. Verify the quadlet exists (return `QUADLET_NOT_FOUND` / 404 if not)
2. Remove the existing quadlet
3. Install the new content (tar-wrapped, `Replace: true`, `ReloadSystemd: true`)

Returns `200` with the updated quadlet DTO (same shape as Get). This is not atomic across the remove/install pair — document that a failure mid-way leaves the quadlet removed.

## Acceptance criteria

- [ ] `QuadletUpdateRequest` DTO with `content` (validated)
- [ ] `Service.Update` performs existence check → remove → install(Replace) of new content
- [ ] `Handler.Update` parses `quadletId` and returns `200` with the updated quadlet
- [ ] `QUADLET_NOT_FOUND` (404) when the source quadlet doesn't exist
- [ ] `VALIDATION_FAILED` (400) on missing `content`
- [ ] `CONNECTION_FAILED` on connection errors
- [ ] Non-atomic behaviour documented in code
- [ ] Service unit test (mock) covering success, not-found, mid-flow-failure, connection-failure
- [ ] Handler unit test covering success, validation failure, not-found

## Blocked by

- 070-03-quadlet-create (install/tar wrapping)
- 070-04-quadlet-delete (remove)

## Technical notes

- Reuses the tar-wrapping from the create slice and the remove call from the delete slice
- `QuadletInstallOptions{Replace: true, ReloadSystemd: true}`
- Contract: `doku/description_api.md:939-961`
