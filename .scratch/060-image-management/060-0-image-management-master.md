---
title: Implement Image Management endpoints using official podman implementation
parent: none
date: 2026-07-30
status: completed
labels:
  - completed
  - images
  - backend
  - feature
  - podman-integration
  - epic
author: vibe-agent
---

## What to build

Implement the Image Management functionality for the Podman Management Web-App using the official podman implementation. The podman repo is checked out under `~/repos/podman`. See `~/repos/podman/pkg/domain/infra/tunnel/images.go` for the reference implementation.

This feature will allow users to list, pull, and remove container images on their registered remote machines through the web interface, leveraging the existing podman service API and the forked podman implementation.

The three image endpoints are already routed in `cmd/server/main.go` (`GET /images`, `POST /images/pull`, `DELETE /images/{imageId}`) but every handler in `internal/domain/images/handler.go` currently returns `501 Not Implemented`. The `ImageService` is a stub with no methods and no DTOs or tests exist.

## Vertical Slices

This feature is broken down into the following vertical slices (tracer bullets). Each slice cuts through service, handler, DTO, and tests end-to-end for one concern.

### 0. Prefactor — make ImageService testable
**Issue**: 060-00-image-service-testability
- Switch `ImageService` to use `machines.ConnectionManagerInterface` (instead of the concrete `*connections.ConnectionManager`)
- Add `NewImageServiceWithConcreteDeps` and an `ImageServiceInterface` for dependency injection
- Add the missing `IMAGE_USED_BY_CONTAINERS` error code

**Status**: ready-for-agent
**Blocked by**: None

### 1. Image List
**Issue**: 060-01-image-list
- `Service.List` using the official podman `ImageEngine.List`
- `ImageListResponse` DTO + mapping from `entities.ImageSummary`
- `Handler.List` implementation
- Service + handler tests

**Status**: ready-for-agent
**Blocked by**: 060-00-image-service-testability

### 2. Image Delete
**Issue**: 060-02-image-delete
- `Service.Delete` using the official podman `ImageEngine.Remove`
- `Handler.Delete` (parse `imageId` path param + `force` query param)
- Error mapping (`IMAGE_NOT_FOUND`, `IMAGE_USED_BY_CONTAINERS`)
- Tests

**Status**: ready-for-agent
**Blocked by**: 060-00-image-service-testability

### 3. Image Pull — basic
**Issue**: 060-03-image-pull-basic
- `Service.Pull` using the official podman `ImageEngine.Pull`
- `ImagePullRequest` DTO (`image`, optional `credentials`, `platform`)
- `Handler.Pull` returning a simple JSON result (pulled image IDs)
- `IMAGE_PULL_FAILED` handling + tests

**Status**: ready-for-agent
**Blocked by**: 060-00-image-service-testability

### 4. Image Pull — streaming + abort
**Issue**: 060-04-image-pull-streaming
- Convert `Handler.Pull` to NDJSON streaming (`progress` / `complete` / `error` events)
- Progress-event DTOs + `operationId`
- Client-disconnect detection → `OPERATION_CANCELLED`
- Tests

**Status**: ready-for-agent
**Blocked by**: 060-03-image-pull-basic

## Technical Context

- **Routing**: Already set up in `cmd/server/main.go` under `/api/machines/{id}/images`
- **Current State**: All three image handlers return "Not implemented" (501); `ImageService` is a stub
- **Podman Reference**: Use `ImageEngine` from `~/repos/podman/pkg/domain/infra/tunnel/images.go`
  - `List` at line 39 — signature `(ctx, entities.ImageListOptions) ([]*entities.ImageSummary, error)`
  - `Pull` at line 115 — signature `(ctx, rawImage string, entities.ImagePullOptions) (*entities.ImagePullReport, error)`
  - `Remove` at line 34 — signature `(ctx, []string, entities.ImageRemoveOptions) (*entities.ImageRemoveReport, []error)`
- **Connection Management**: Use existing `ConnectionManager.GetConnection()` to establish connections to remote machines (same pattern as `ContainerService`)
- **Authentication**: User authentication already handled via JWT middleware
- **API Contract**: See `doku/description_api.md` (Image Endpoints, lines 693-829) for request/response shapes, error codes, and the streaming format for pull

## Acceptance criteria

- [ ] Image list endpoint returns actual image data from remote machines
- [ ] Image pull endpoint pulls images from a registry
- [ ] Image pull streams progress as NDJSON and supports client abort
- [ ] Image delete endpoint removes images, respecting the `force` flag
- [ ] Proper error handling for connection failures and API errors
- [ ] Consistent API response format with other endpoints (containers)
- [ ] Comprehensive test coverage following the containers test patterns

## Blocked by

None - can start immediately with slice 0 (prefactor)
