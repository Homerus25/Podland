---
title: Secret Inspect endpoint with admin-only value reveal
parent: 160-secrets-management
date: 2026-08-27
status: closed
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

Implement `GET /api/machines/{id}/secrets/{secretId}?reveal=true|false`. Podman *can* return secret values on inspect (`InspectOptions.WithShowSecret(true)` makes the abi layer run `LookupSecretData`, `~/repos/podman/pkg/domain/infra/abi/secrets.go:80-83`, filling `SecretData`). Per the permission ruling, that power is **admin-only**:

- `Service.Get(ctx, userID, machineID, secretID, reveal)` → `secrets.Inspect(connCtx, secretID, opts)`; opts sets `WithShowSecret(true)` **only** when `reveal && caller level == admin`
- `secretId` URL param accepts name or ID (podman resolves both)
- Response DTO: list DTO fields plus `SecretData string` **present only on an admin reveal** — omit the field entirely otherwise (marshal `omitempty`, and only populate it from a reveal call; never from list)
- When `reveal=true` but the caller is below `admin`, return 403 `ACCESS_DENIED` — do **not** silently degrade to metadata, so the UI can distinguish "no permission" from "no value"
- The service method needs the caller's level: fetch via the repo `GetPermission` added in 160-00 (handler passes userID; service or handler resolves level — follow wherever 160-00 placed the ordering helper)

Redaction discipline (hard requirements):

- `SecretData` must never appear in logs at any level; no debug printing of the value anywhere
- Reveal calls may be logged as events ("user X revealed secret Y") without the value

## Acceptance criteria

- [x] Inspect returns metadata for `read`+ users with no value field in the JSON
- [x] `?reveal=true` as `admin`/owner returns the value in `SecretData`
- [x] `?reveal=true` as `read`/`write` returns 403 `ACCESS_DENIED`
- [x] Unknown secret maps to 404 `SECRET_NOT_FOUND` (bindings 404 error model detection, same approach as tunnel layer)
- [x] Service unit tests: metadata-only, reveal-on, level enforcement; handler unit tests for all four plus connection failure
- [x] Grep-check the slice: no test or code path logs `SecretData`

## Blocked by

- 160-01-secret-list
- 160-00-level-aware-access

## Technical notes

- Bindings: `secrets.Inspect(ctx, nameOrID, *InspectOptions)` (`~/repos/podman/pkg/bindings/secrets/secrets.go:33`); `InspectOptions.WithShowSecret(bool)` (`types.go`)
- Podman 404 detection: `errorhandling.ErrorModel` with `ResponseCode == 404` — see the tunnel reference `~/repos/podman/pkg/domain/infra/tunnel/secrets.go:37-44`
- Mount as `r.Route("/{secretId}", ...)` or `r.Get("/secrets/{secretId}", ...)` consistent with quadlet style in `app.go`

## Implementation notes (2026-08-28)

- The level check lives in `SecretService.Get`, before the connection lease is taken: the reveal is a query param, so route-level `RequireMachinePermission` (fixed level per route) cannot express "admin only when ?reveal=true". A below-admin reveal is denied without podman ever being asked; a failing `GetPermission` lookup is a 500, mirroring the middleware's never-guess contract.
- `SecretService` gained a narrow `accessPermissionGetter` dependency (the `GetPermission` slice, mirroring the middleware's local interface); `NewSecretService` now takes the access repo.
- The reveal audit event (`secret value revealed`, userID/machineID/secretID, never the value) is logged by the service on every successful reveal.
- 404 detection uses `errors.As(*errorhandling.ErrorModel)` + `ResponseCode == 404` (tunnel-layer approach), not string matching; any other podman error stays a wrapped non-AppError the handler 500s.
- Deny-before-404 ordering: a read-level caller revealing an unknown secret gets 403, not 404 (fail-closed).
- Routes mounted quadlet-style: `r.Route("/secrets", ...)` → `r.Route("/{secretId}", ...)` → `r.Get("/", secretHandler.Get)`.
