---
title: Implement Container List using official podman implementation
parent: none
date: 2026-07-04
status: completed
labels:
  - completed
  - containers
  - backend
  - feature
  - podman-integration
  - epic
author: vibe-agent
---

## What to build

Implement the Container List functionality for the Podman Management Web-App using the official podman implementation. The podman repo is checked out under ~/repos/podman. See ~/repos/podman/pkg/domain/infra/tunnel/containers.go for the reference implementation.

This feature will allow users to list containers on their registered remote machines through the web interface, leveraging the existing podman service API and the forked podman implementation.

## Vertical Slices

This feature is broken down into the following vertical slices (tracer bullets):

### 1. Backend Service Container List Method
**Issue**: 030-1-backend-service-container-list-method
- Add `List` method to `ContainerService`
- Use ConnectionManager to get podman connection to target machine
- Call official podman `ContainerList` implementation
- Handle connection and podman API errors

**Status**: ready-for-agent
**Blocked by**: None

### 2. Backend Handler Container List Endpoint  
**Issue**: 030-2-backend-handler-container-list-endpoint
- Implement `ContainerHandler.List` method
- Parse query parameters for list options
- Call ContainerService.List method
- Return JSON response with container list

**Status**: pending
**Blocked by**: 030-1-backend-service-container-list-method

### 3. API Response DTO and Mapping
**Issue**: 030-3-api-response-dto-and-mapping
- Create `ContainerListResponse` DTO
- Create mapping from podman `ListContainer` entities to API responses
- Handle field filtering and serialization

**Status**: pending  
**Blocked by**: 030-1-backend-service-container-list-method

### 4. Integration Tests for Container List
**Issue**: 030-4-integration-tests-container-list
- Unit tests for ContainerService.List method
- Unit tests for ContainerHandler.List method
- Tests for DTO mapping functionality
- Tests for error handling scenarios

**Status**: pending
**Blocked by**: 030-1-backend-service-container-list-method, 030-2-backend-handler-container-list-endpoint, 030-3-api-response-dto-and-mapping

## Technical Context

- **Routing**: Already set up in `main.go` at `/api/machines/{id}/containers` with GET method
- **Current State**: `ContainerHandler.List` currently returns "Not implemented"
- **Podman Reference**: Use `ContainerEngine.ContainerList` from `~/repos/podman/pkg/domain/infra/tunnel/containers.go:870`
- **Connection Management**: Use existing `ConnectionManager.GetConnection()` to establish connections to remote machines
- **Authentication**: User authentication already handled via JWT middleware

## Acceptance criteria

- [ ] Container list endpoint returns actual container data from remote machines
- [ ] Supports filtering and listing options (all, filters, etc.)
- [ ] Proper error handling for connection failures and API errors
- [ ] Consistent API response format with other endpoints
- [ ] Comprehensive test coverage

## Blocked by

None - can start immediately with slice 1