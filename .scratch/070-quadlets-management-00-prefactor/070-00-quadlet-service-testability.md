---
title: Prefactor — make QuadletService testable
parent: 070-quadlets-management
date: 2026-08-02
status: completed
labels:
  - completed
  - quadlets
  - backend
  - refactor
  - testability
author: vibe-agent
---

## Parent

070-quadlets-management — Implement Quadlet management endpoints (backend) against the podman service API

## What to build

Refactor the `QuadletService` so it is testable in isolation, matching the pattern already established by `ImageService` (issue 060-00) and `ContainerService`. This is a "make the change easy, then make the easy change" prefactor that unblocks testable implementation of the list/get/create/delete/update slices.

Currently `QuadletService` (in `internal/domain/quadlets/service.go`) depends on the **concrete** `*connections.ConnectionManager`. This prevents unit tests from injecting a mock connection manager, so the upcoming endpoint slices could not ship with proper unit tests. `ImageService` already solved this by depending on `machines.ConnectionManagerInterface`.

The handler also currently takes the concrete `*QuadletService`; switch it to depend on a `QuadletServiceInterface` so handlers can be unit-tested with a mock (mirroring `ImageHandler`).

This slice should:

- Change `QuadletService.connManager` from `*connections.ConnectionManager` to `machines.ConnectionManagerInterface`
- Add a `NewQuadletServiceWithConcreteDeps(connManager *connections.ConnectionManager)` constructor that wraps the concrete manager in an adapter (mirroring `images.NewImageServiceWithConcreteDeps` / `concreteConnectionManager`)
- Define a `QuadletServiceInterface` in the quadlets package, ready for handlers to depend on (method signatures will be added as slices 1–5 are implemented; for this slice define it empty or with the methods you can already foresee — it must compile)
- Change `QuadletHandler.service` to `QuadletServiceInterface` and add a compile-time check `var _ QuadletServiceInterface = (*QuadletService)(nil)`
- Update the wiring in `cmd/server/main.go` to call `NewQuadletServiceWithConcreteDeps(connectionManager)` (currently calls `NewQuadletService(connectionManager)`)

No new endpoint behavior is added — all handlers still return `501`. The build must remain green and existing behavior unchanged.

## Acceptance criteria

- [ ] `QuadletService` depends on `machines.ConnectionManagerInterface`, not the concrete `*connections.ConnectionManager`
- [ ] `NewQuadletServiceWithConcreteDeps` exists and wraps the concrete connection manager in an adapter (same shape as `images.NewImageServiceWithConcreteDeps`)
- [ ] A `QuadletServiceInterface` is defined in the quadlets package, ready for handler dependency injection
- [ ] `QuadletHandler.service` is typed as `QuadletServiceInterface`
- [ ] `cmd/server/main.go` wiring updated to `NewQuadletServiceWithConcreteDeps(connectionManager)` and still compiles
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` is clean
- [ ] Existing tests still pass (`go test ./...`)

## Blocked by

None - can start immediately

## Technical notes

- The adapter + interface pattern to mirror lives in `internal/domain/images/service.go` (`concreteConnectionManager`, `ImageServiceInterface`, `NewImageServiceWithConcreteDeps`)
- The shared interface lives in `internal/domain/machines/service_interfaces.go` (`ConnectionManagerInterface` with `GetConnection(ctx, machineID, userID) (any, error)` and `Release(machineID)`)
- The mock for tests already exists: `internal/testutils/mock_services.go` `MockConnectionManager`
- `QuadletServiceInterface` will gain method signatures as later slices are implemented; for this slice it must at minimum compile
