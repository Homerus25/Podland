---
title: Secret Delete endpoint
parent: 160-secrets-management
date: 2026-08-27
status: open
labels:
  - ready-for-agent
  - backend
  - secrets
  - feature
  - podman-integration
author: vibe-agent
---

## Parent

160-secrets-management — Implement Secret management endpoints (backend) against the podman service API

## What to build

Implement `DELETE /api/machines/{id}/secrets/{secretId}` (write-level route via `RequireMachinePermission(write)` from 160-00). Bindings: `secrets.Remove(connCtx, nameOrID)` (`~/repos/podman/pkg/bindings/secrets/secrets.go:55`).

- `Service.Delete(ctx, userID, machineID, secretID)` → `secrets.Remove`; return `204 No Content` on success
- `secretId` accepts name or ID
- Error mapping:
  - podman 404 → 404 `SECRET_NOT_FOUND`
  - secret in use by a container → 409 `SECRET_IN_USE` (detect from the podman error model message; podman refuses to remove secrets referenced by containers)
  - anything else → 502 `CONNECTION_FAILED` or 500 fallback per quadlets idioms

## Acceptance criteria

- [ ] `DELETE` gated at `write`: `read` → 403, `write`/`admin` → 204
- [ ] Unknown secret → 404 `SECRET_NOT_FOUND`
- [ ] In-use secret → 409 `SECRET_IN_USE` with podman's message surfaced
- [ ] Service unit tests (success, 404, in-use) and handler tests (status codes incl. 403/404-machine)
- [ ] List invalidation contract noted for the frontend series (170) — response body is empty on 204

## Blocked by

- 160-01-secret-list
- 160-00-level-aware-access

## Technical notes

- `Remove` returns a bare error (no report struct) — distinguish 404 via `errorhandling.ErrorModel.ResponseCode`, same detection as the tunnel layer (`~/repos/podman/pkg/domain/infra/tunnel/secrets.go:55-70`)
- No force option in bindings; do not invent one
