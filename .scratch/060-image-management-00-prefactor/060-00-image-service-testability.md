---
title: Prefactor — make ImageService testable
parent: 060-image-management
date: 2026-07-30
status: completed
labels:
  - completed
  - images
  - backend
  - refactor
  - testability
author: vibe-agent
---

## Parent

060-image-management — Implement Image Management endpoints using official podman implementation

## What to build

Refactor the `ImageService` so it is testable in isolation, matching the pattern already established by `ContainerService`. This is a "make the change easy, then make the easy change" prefactor that unblocks testable implementation of the list, pull, and delete endpoints.

Currently `ImageService` (in `internal/domain/images/service.go`) depends on the **concrete** `*connections.ConnectionManager`. This prevents unit tests from injecting a mock connection manager, so the upcoming endpoint slices could not ship with proper unit tests. `ContainerService` already solved this by depending on `machines.ConnectionManagerInterface`.

This slice should:

- Change `ImageService.connManager` from `*connections.ConnectionManager` to `machines.ConnectionManagerInterface`
- Add a `NewImageServiceWithConcreteDeps(connManager *connections.ConnectionManager)` constructor that wraps the concrete manager in the adapter (mirroring `containers`' `concreteConnectionManager`)
- Define an `ImageServiceInterface` in the images package that declares the methods the handlers will call, so handlers can depend on the interface and be unit-tested with a mock
- Update the handler constructor / wiring in `cmd/server/main.go` if the `NewImageService` call site needs adjusting
- Add the missing `IMAGE_USED_BY_CONTAINERS` error code to `internal/errors/errors.go` (it is referenced by the API contract for the delete endpoint but does not exist yet)

No new endpoint behavior is added — all three handlers still return `501`. The build must remain green and existing behavior unchanged.

## Acceptance criteria

- [ ] `ImageService` depends on `machines.ConnectionManagerInterface`, not the concrete `*connections.ConnectionManager`
- [ ] `NewImageServiceWithConcreteDeps` exists and wraps the concrete connection manager in an adapter (same shape as `containers.NewContainerServiceWithConcreteDeps`)
- [ ] An `ImageServiceInterface` is defined in the images package, ready for handler dependency injection
- [ ] `cmd/server/main.go` still compiles and the image handler/service wiring is correct
- [ ] `ErrImageUsedByContainers` (`IMAGE_USED_BY_CONTAINERS`, HTTP 400) is added to `internal/errors/errors.go`
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] Existing tests still pass (`go test ./...`)

## Blocked by

None - can start immediately

## Technical notes

- The adapter + interface pattern to mirror lives in `internal/domain/containers/service.go` (`concreteConnectionManager`, `ContainerServiceInterface`, `NewContainerServiceWithConcreteDeps`)
- The shared interface lives in `internal/domain/machines/service_interfaces.go` (`ConnectionManagerInterface` with `GetConnection(ctx, machineID, userID) (any, error)` and `Release(machineID)`)
- `ImageServiceInterface` will gain method signatures as slices 1–3 are implemented; for this slice, define it with the methods you can already foresee or leave it ready to extend — but it must compile
- The mock for tests already exists: `internal/testutils/mock_services.go` `MockConnectionManager`
- `IMAGE_USED_BY_CONTAINERS` should map to HTTP 400 per `doku/description_api.md` line 823
