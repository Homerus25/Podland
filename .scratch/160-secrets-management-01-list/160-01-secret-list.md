---
title: Secret List endpoint
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

Implement `GET /api/machines/{id}/secrets` end-to-end. This is the foundation slice — it establishes the `internal/domain/secrets` package (service interface + compile-time check, `machines.ConnectionManagerInterface` dependency, handler, DTO file), the error sentinels, and the route registration in `internal/app/app.go` that all later slices extend.

Follow the `internal/domain/quadlets/` structure (`service.go` interface + `NewSecretService(connManager)`, `handler.go`, `dto.go`) and call the bindings package directly on the tunnel context, mirroring `quadlets.Install` (`internal/domain/quadlets/service.go:174`):

- `Service.List(ctx, userID, machineID)` → `connManager.GetConnection` → `secrets.List(connCtx, nil)` → map each `entities.SecretInfoReport` to the response DTO
- DTO (metadata only — **never** include `SecretData`): `id`, `name` (`Spec.Name`), `driver` (`Spec.Driver`), `createdAt`, `updatedAt`, `labels` (`Spec.Labels`, omit when empty)
- Handler wired at `r.Get("/secrets", secretHandler.List)` inside the existing `RequireMachineAccess` subtree (`internal/app/app.go:206-263`); subtree membership is the `read` gate
- Add the `// Secrets` sentinel block to `internal/errors/errors.go`: `SECRET_NOT_FOUND` (404), `SECRET_ALREADY_EXISTS` (409), `SECRET_IN_USE` (409), `SECRET_CREATION_FAILED` (500) — created here so later slices don't touch the file

## Acceptance criteria

- [x] `SecretServiceInterface` + compile-time assertion, constructor taking only `ConnectionManagerInterface`
- [x] `Handler.List` returns `200` with a JSON array of secret metadata
- [x] Response never contains a value field; `SecretData` is dropped in mapping
- [x] `CONNECTION_FAILED` on tunnel errors; handler falls back to `ErrInternalError` for non-`AppError` returns (quadlets handler idiom)
- [x] Service unit test with mock `ConnectionManagerInterface` asserting the bindings call and mapping (skip empty-label fields)
- [x] Handler unit test covering success, empty list, connection failure, missing machine (404), no-access user (403)

## Blocked by

- 160-00-level-aware-access

## Technical notes

- Bindings: `secrets.List(ctx, *ListOptions)` → `[]*entities.SecretInfoReport` (`~/repos/podman/pkg/bindings/secrets/secrets.go:13`); import `go.podman.io/podman/v6/pkg/bindings/secrets`
- Entity: `SecretInfoReport{ID, CreatedAt, UpdatedAt, Spec{Name, Driver, Labels}, SecretData,omitempty}` — `~/repos/podman/pkg/domain/entities/types/secrets.go`, aliased at `entities/secrets.go:35`
- Wire the service + handler in `app.go` alongside the quadlets pair (`internal/app/app.go:146-159`)
- No persistence in our DB; no migration in this series

## Implementation notes (2026-08-28)

- The bindings-call/mapping assertions use a fake podman HTTP server (`httptest` answering `/_ping` + `/libpod/secrets/json`) reached through a stub `ConnectionManagerInterface` returning a `bindings.NewConnection` tcp context — the deepest seam that can observe the real bindings invocation in a unit test. The `testutils` mock backs the connection-failure test.
- `driver` maps `Spec.Driver.Name` (the entity's `Spec.Driver` is a `SecretDriverSpec` struct, not a string).
- 404/403 coverage mounts the route under `RequireMachineAccess` exactly as `app.go` does, plus a read-level pass-through test pinning "subtree membership is the read gate", and an app-level test pinning JWT protection of the registered route.
