---
title: Image List endpoint — GET /api/machines/{id}/images
parent: 060-image-management
date: 2026-07-30
status: completed
labels:
  - completed
  - images
  - backend
  - service-layer
  - podman-integration
author: vibe-agent
---

## Parent

060-image-management — Implement Image Management endpoints using official podman implementation

## What to build

Implement the **list images** endpoint end-to-end so that an authenticated user can list all images on a registered machine. This is the simplest image slice and the first real tracer bullet through the podman image API.

The user calls `GET /api/machines/{id}/images` and receives a JSON array of images present on that machine. The handler authenticates via context, reads the machine ID from the URL, parses list options from query params, calls the service, and returns mapped DTOs.

This slice covers: the `Service.List` method using the official podman tunnel `ImageEngine.List`, the `ImageListResponse` DTO and mapping from `entities.ImageSummary`, the `Handler.List` implementation, and unit tests for both service and handler.

## Acceptance criteria

- [ ] `Service.List(ctx, userID, machineID, opts)` returns `[]*entities.ImageSummary` from the official podman `ImageEngine.List`
- [ ] Service uses `ConnectionManager.GetConnection()` to reach the target machine and calls `Release()` when done (defer), following the `ContainerService.List` pattern
- [ ] An `ImageListResponse` DTO exists in the images package with fields per the API contract (`id`, `repoTags`, `repoDigests`, `parentID`, `createdAt`, `size`, plus optional `os`, `architecture`, `labels`)
- [ ] A mapping function converts `entities.ImageSummary` to `ImageListResponse`
- [ ] `Handler.List` extracts the user ID from context and machine ID from the chi URL param, returning 401/400 appropriately (mirrors `ContainerHandler.List`)
- [ ] `Handler.List` parses the `all` and `filters` query parameters and passes them as `entities.ImageListOptions`
- [ ] On podman/connection errors the handler responds with the structured error format via `errors.RespondWithError` / `RespondWithHTTPError`
- [ ] Unit tests for `Service.List` cover: connection failure, successful invocation shape (using `testutils.MockConnectionManager`)
- [ ] Unit tests for `Handler.List` cover: missing/empty user ID, missing machine ID, service-error path
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` all pass

## Blocked by

- 060-00-image-service-testability (needed for the testable `ImageServiceInterface` and mock injection)

## Technical notes

- Reference podman implementation: `~/repos/podman/pkg/domain/infra/tunnel/images.go:39` — `func (ir *ImageEngine) List(_ context.Context, opts entities.ImageListOptions) ([]*entities.ImageSummary, error)`
- Construct the engine as `&tunnel.ImageEngine{ClientCtx: connCtx}` exactly like containers does with `tunnel.ContainerEngine` (`internal/domain/containers/service.go:79`)
- `ImageEngine` uses `ir.ClientCtx` to call the bindings layer, so the connection context returned by `ConnectionManager` is what you assign to `ClientCtx`
- Handler pattern to follow: `internal/domain/containers/handler.go` `List` (lines 31-146) for context extraction, query parsing, error handling, and JSON encoding
- API response shape: `doku/description_api.md` lines 704-720
- Query params per API contract: `all` (bool), `filters` (object) — `entities.ImageListOptions` has `All` and `Filter []string`
