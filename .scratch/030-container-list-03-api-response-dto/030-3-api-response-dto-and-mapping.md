---
title: API Response DTO and Mapping for Container List
parent: implement Container List using official podman implementation
date: 2026-07-04
status: completed
labels:
  - containers
  - backend
  - api-layer
  - dto
  - serialization
author: vibe-agent
---

## Parent

Implement Container List by using the official podman implementation. The podman repo is checked out under ~/repos/podman. See ~/repos/podman/pkg/domain/infra/tunnel/containers.go

## What to build

Create API response Data Transfer Objects (DTOs) for container listing and implement mapping between podman entities and API responses. This includes:

- Create `ContainerListResponse` struct that represents a container in the API response
- Create `ContainerListResult` struct that wraps the list of containers with metadata
- Implement mapping functions to convert `entities.ListContainer` to `ContainerListResponse`
- Handle field filtering and serialization for the API response
- Ensure proper JSON tags and field naming conventions

## Acceptance criteria

- [x] `ContainerListResponse` DTO created with appropriate fields
- [x] `ContainerListResult` wrapper struct created
- [x] Mapping function from `entities.ListContainer` to `ContainerListResponse` implemented
- [x] Proper JSON tags applied to all DTO fields
- [x] Handles null/empty values appropriately
- [x] Follows existing DTO patterns in the codebase (similar to MachineResponse)
- [x] Includes essential container fields: ID, Names, Image, State, Status, Created, etc.

## Blocked by

- 030-1-backend-service-container-list-method

## Technical notes

- Look at existing DTO patterns in `machines/handler.go` (MachineResponse struct)
- The podman `ListContainer` entity contains many fields - select the most relevant for API response
- Consider creating a separate DTO file in the containers package
- Use appropriate JSON field names that match frontend expectations
- The mapping should be bidirectional where possible for consistency