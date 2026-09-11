---
title: Implement Secret management endpoints (backend) against the podman service API
parent: none
date: 2026-08-27
status: ready-for-agent
labels:
  - secrets
  - backend
  - feature
  - podman-integration
  - epic
author: vibe-agent
---

## What to build

Implement the 4 secret management endpoints under `/api/machines/{id}/secrets`. Nothing exists today: no `internal/domain/secrets` package, no routes in `internal/app/app.go`, no DTOs, no error sentinels, no contract section in `doku/`. This epic is **backend-only**; the UI is a separate series (`170-secrets-ui`).

The podman bindings package `go.podman.io/podman/v6/pkg/bindings/secrets` (checked out under `~/repos/podman`) exposes exactly 5 ops: `List`, `Inspect`, `Create`, `Remove`, `Exists`. Secrets are **not persisted** in our database — the service proxies podman directly, matching `ContainerService`/`ImageService`/`QuadletService`.

### Ruling: permission model (grilling 2026-08-27)

Secrets reuse the existing `machine_access` levels — **no schema change, no new grant table**. The owner already controls who has any access at all (sharing is admin-only), which satisfies "only the owner can allow other users to see and/or change secrets":

- **read** — see secret **metadata** (name, ID, created, driver)
- **write** — additionally create and delete secrets
- **admin** — additionally **reveal secret values** (`?reveal=true` → podman `showsecret`)

Consequence: `RequireMachineAccess` is **boolean-only** today (`HasAccess`, `models/machine_access_repository.go:134-144`); any access row grants full subtree use, so a `read` user can currently mutate containers/images/quadlets. This epic introduces level-aware enforcement and **retrofits it onto all existing mutating routes** (user decision), fixing that inconsistency in the same pass.

### Ruling: capability scope (v1)

- v1 ops: **List, Inspect (with admin-only reveal), Create (name + value), Delete**.
- No update endpoint — podman secrets are immutable; a replace flow (delete + recreate) is a conscious deferral.
- No labels / driver / driver-opts on create (defaults: file driver). Follow-up material.
- The secret **value** travels exactly once (create request), is never persisted by us, never logged, and is only ever returned by the reveal path (admin).

## Vertical Slices

### 0. Prefactor — level-aware machine access middleware + retrofit
**Issue**: 160-00-level-aware-access
- `GetPermission(machineID, userID)` repo method (level string, '' when absent)
- `RequireMachinePermission(accessRepo, required)` middleware with read < write < admin ordering
- Retrofit onto all mutating container/image/quadlet routes; `read` users get 403
- Tests

**Status**: ready-for-agent
**Blocked by**: None

### 1. Secret List
**Issue**: 160-01-secret-list
- Establishes `internal/domain/secrets` package (service interface + connManager pattern), DTO mapping, error sentinels, route registration
- `Service.List` via bindings `secrets.List`; `read` level
- Tests

**Status**: ready-for-agent
**Blocked by**: 160-00

### 2. Secret Inspect + admin-only reveal
**Issue**: 160-02-secret-inspect-reveal
- `Service.Get` via bindings `secrets.Inspect`; `?reveal=true` requires `admin` and sets `WithShowSecret(true)`
- Value redaction discipline: `SECRET_VALUE` field present only on reveal; never logged
- Tests

**Status**: ready-for-agent
**Blocked by**: 160-01

### 3. Secret Create
**Issue**: 160-03-secret-create
- `Service.Create` via bindings `secrets.Create` (value as body reader); `write` level
- Validation: name pattern, value 1..1 MiB, **value never trimmed**
- Tests

**Status**: ready-for-agent
**Blocked by**: 160-01

### 4. Secret Delete
**Issue**: 160-04-secret-delete
- `Service.Delete` via bindings `secrets.Remove`; `write` level
- `SECRET_NOT_FOUND` / in-use error mapping
- Tests

**Status**: ready-for-agent
**Blocked by**: 160-01

### 5. Integration tests + API contract docs
**Issue**: 160-05-integration-docs
- Secrets slice in the 080 integration harness (real podman)
- Permission tripwires: read → 403 on create/delete/reveal; write → 403 on reveal
- `doku/description_secrets_api.md` + link from `doku/description_api.md`

**Status**: ready-for-agent
**Blocked by**: 160-01..160-04

## Technical Context

- **Routing**: register inside the existing `RequireMachineAccess` subtree in `internal/app/app.go:206-263`:
  `GET/POST /machines/{id}/secrets`, `GET/DELETE /machines/{id}/secrets/{secretId}` (id = name or ID; podman resolves both)
- **Bindings reference** (`~/repos/podman/pkg/bindings/secrets/secrets.go`):
  - `List(ctx, *ListOptions)` → `[]*entities.SecretInfoReport` (line 13)
  - `Inspect(ctx, nameOrID, *InspectOptions)` → `*entities.SecretInfoReport` (line 33; `InspectOptions.WithShowSecret(bool)`)
  - `Remove(ctx, nameOrID)` → error (line 55)
  - `Create(ctx, io.Reader, *CreateOptions)` → `*entities.SecretCreateReport` (line 71; `WithName`, value = raw body reader)
  - `Exists(ctx, nameOrID)` → bool (line 92)
- **Entity**: `entities.SecretInfoReport{ID, CreatedAt, UpdatedAt, Spec{Name, Driver, Labels}, SecretData,omitempty}` (`~/repos/podman/pkg/domain/entities/types/secrets.go`, aliased at `entities/secrets.go:35`). `SecretData` is populated **only** when inspect runs with `ShowSecret` (abi: `LookupSecretData`, `pkg/domain/infra/abi/secrets.go:80-83`).
- **Error sentinels**: none exist — add a `// Secrets` block in `internal/errors/errors.go` (~line 110): `SECRET_NOT_FOUND` (404), `SECRET_ALREADY_EXISTS` (409), `SECRET_IN_USE` (409), `SECRET_CREATION_FAILED` (500)
- **Connection management**: existing `ConnectionManagerInterface.GetConnection(machineID, userID)` / `Release(machineID)` (defined `internal/domain/machines/service_interfaces.go:9-12`)
- **Established patterns**: mirror `internal/domain/quadlets/` (bindings-level calls like `quadlets.Install` in `service.go:174` are the precedent for calling `secrets.*` directly on the tunnel context)
- **Podman events**: podman emits `secret` events (`libpod/events.go:197`), but the frontend has no event bus yet — list freshness is React Query invalidation in 170

## Acceptance criteria

- [ ] Level-aware middleware exists and gates all mutating routes app-wide; `read` users get 403 `ACCESS_DENIED` on container/image/quadlet writes
- [ ] Secret list returns metadata from the remote machine for `read`+ users
- [ ] Inspect returns detail; `?reveal=true` returns the value **only** for `admin`; value never appears in logs or non-reveal responses
- [ ] Create installs a secret from name + value; 409 on name conflict; value unmodified (no trim)
- [ ] Delete removes a secret; in-use secrets surface `SECRET_IN_USE`
- [ ] Proper error handling for connection failures, consistent with containers/images/quadlets
- [ ] Unit tests (testutils mocks) + integration tests (080 harness) + contract docs

## Blocked by

None - can start immediately with slice 0 (prefactor)
