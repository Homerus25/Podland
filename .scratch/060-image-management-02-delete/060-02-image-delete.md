---
title: Image Delete endpoint — DELETE /api/machines/{id}/images/{imageId}
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

Implement the **delete image** endpoint end-to-end so an authenticated user can remove an image from a registered machine. Deleting an image in use by containers is guarded by a `force` flag.

The user calls `DELETE /api/machines/{id}/images/{imageId}?force=true`. The handler authenticates via context, reads the machine ID and image ID from the URL, reads the optional `force` query param, calls the service, and returns `204 No Content` on success.

This slice covers: the `Service.Delete` method using the official podman tunnel `ImageEngine.Remove`, the `Handler.Delete` implementation, error mapping (including the `force`-guarded in-use case), and unit tests.

## Acceptance criteria

- [ ] `Service.Delete(ctx, userID, machineID, imageID, force) error` calls the official podman `ImageEngine.Remove` with `entities.ImageRemoveOptions{Force: force}`
- [ ] Service uses `ConnectionManager.GetConnection()` / `Release()` following the `ContainerService.Delete` pattern
- [ ] When podman reports the image does not exist, the service returns `errors.ErrImageNotFound` (404, `IMAGE_NOT_FOUND`)
- [ ] When the image is in use by containers and `force` is false, the service returns `errors.ErrImageUsedByContainers` (400, `IMAGE_USED_BY_CONTAINERS`) — this error code is added in the prefactor slice
- [ ] `Handler.Delete` extracts user ID from context, machine ID and image ID (`chi.URLParam` `imageId`) from the URL, and parses the `force` query param (default false), mirroring `ContainerHandler.Delete`
- [ ] On success the handler responds `204 No Content` with no body
- [ ] Errors are returned in the structured error format
- [ ] Unit tests for `Service.Delete` cover: connection failure, image-not-found mapping, missing parameters
- [ ] Unit tests for `Handler.Delete` cover: missing user ID, missing machine ID, missing image ID, service-error path
- [ ] `go build ./...`, `go vet ./...`, and `go test ./...` all pass

## Blocked by

- 060-00-image-service-testability (needed for the `ErrImageUsedByContainers` error code and testable service)

## Technical notes

- Reference podman implementation: `~/repos/podman/pkg/domain/infra/tunnel/images.go:34` — `func (ir *ImageEngine) Remove(_ context.Context, imagesArg []string, opts entities.ImageRemoveOptions) (*entities.ImageRemoveReport, []error)`. Note it returns `(*report, []error)` — a per-image error slice, not a single error.
- Pass the image ID as `[]string{imageID}` and inspect both the returned error and the per-image error slice for the not-found / in-use conditions
- Handler pattern to follow: `internal/domain/containers/handler.go` `Delete` (lines 347-390) for context extraction, `force` query parsing (`strconv.ParseBool`), and the 204 response
- Service error-mapping pattern to follow: `internal/domain/containers/service.go` `Delete` (lines 310-352), which maps `strings.Contains(err.Error(), "no such container")` to `ErrContainerNotFound`
- API response shape and error codes: `doku/description_api.md` lines 810-829
