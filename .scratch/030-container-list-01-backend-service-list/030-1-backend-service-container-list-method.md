---
title: Backend Service Container List Method
parent: implement Container List using official podman implementation
date: 2026-07-04
status: completed
labels:
  - completed
  - containers
  - backend
  - service-layer
  - podman-integration
author: vibe-agent
---

## Parent

Implement Container List by using the official podman implementation. The podman repo is checked out under ~/repos/podman. See ~/repos/podman/pkg/domain/infra/tunnel/containers.go

## What to build

Add a `List` method to `ContainerService` that provides the business logic for listing containers on a specific machine. This method should:

- Accept context, userID, machineID, and list options as parameters
- Use ConnectionManager to get a podman connection to the specified machine
- Call the official podman `ContainerList` method from the tunnel implementation
- Handle connection errors and podman API errors appropriately
- Return a list of container entities from podman

The method should leverage the existing `ContainerEngine.ContainerList` method from the official podman implementation at `~/repos/podman/pkg/domain/infra/tunnel/containers.go:870` which uses `containers.List()` from the bindings package.

## Acceptance criteria

- [ ] `List` method added to `ContainerService` with proper signature
- [ ] Uses ConnectionManager to establish connection to target machine
- [ ] Calls official podman `ContainerList` implementation
- [ ] Passes through list options (filters, all containers, etc.) to podman API
- [ ] Returns `[]entities.ListContainer` from podman
- [ ] Handles connection errors appropriately
- [ ] Handles podman API errors appropriately
- [ ] Follows existing code style and patterns in the service layer

## Blocked by

None - can start immediately

## Technical notes

- Use the existing `ConnectionManager.GetConnection()` method to get podman bindings connection
- The official podman `ContainerList` method signature: `func (ic *ContainerEngine) ContainerList(_ context.Context, opts entities.ContainerListOptions) ([]entities.ListContainer, error)`
- Import the podman entities from the forked repo: `go.podman.io/podman/v6/pkg/domain/entities`
- The connection returned by ConnectionManager is a `*bindings.Connection` which can be used as the context for podman operations