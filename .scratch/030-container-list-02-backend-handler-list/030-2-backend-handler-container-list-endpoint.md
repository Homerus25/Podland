---
title: Backend Handler Container List Endpoint
parent: implement Container List using official podman implementation
date: 2026-07-04
status: completed
labels:
  - containers
  - backend
  - handler-layer
  - api-endpoint
  - podman-integration
author: vibe-agent
---

## Parent

Implement Container List by using the official podman implementation. The podman repo is checked out under ~/repos/podman. See ~/repos/podman/pkg/domain/infra/tunnel/containers.go

## What to build

Implement the `ContainerHandler.List` method to handle HTTP requests for listing containers on a specific machine. This method should:

- Extract userID from request context for authentication/authorization
- Extract machineID from URL parameters
- Parse query parameters for list options (all, filters, size, etc.)
- Call the ContainerService.List method with appropriate parameters
- Return JSON response with the container list
- Handle errors and return appropriate HTTP status codes

The endpoint is already routed in main.go at `/api/machines/{id}/containers` with GET method.

## Acceptance criteria

- [x] `ContainerHandler.List` method implemented
- [x] Extracts and validates userID from context
- [x] Extracts and validates machineID from URL parameters
- [x] Parses query parameters for container list options
- [x] Calls `ContainerService.List` with context, userID, machineID, and options
- [x] Returns JSON response with container list
- [x] Returns appropriate HTTP status codes for different error scenarios
- [x] Follows existing error handling patterns in the codebase

## Blocked by

- 030-1-backend-service-container-list-method

## Technical notes

- Use existing `contextutils.GetUserIDFromContext()` for user extraction
- Use `chi.URLParam(r, "id")` for machineID extraction (note: this is the machine ID, not container ID)
- Query parameters to support: `all`, `filters`, `size`, `namespace`, `sync`, `external`, `last`
- Return HTTP 200 with container list on success
- Return HTTP 400 for invalid parameters
- Return HTTP 401 for unauthorized access
- Return HTTP 404 if machine not found
- Return HTTP 500 for internal errors