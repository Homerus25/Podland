# 032 - Implement Container Get Endpoint

**Status**: completed

## Parent
030-container-list

## What to build
Implement the container get endpoint that retrieves detailed information about a specific container on a machine. This endpoint should return comprehensive container details including configuration, state, and metadata.

## Acceptance criteria
- [ ] ContainerService.Get method added to fetch a specific container by ID from Podman API
- [ ] ContainerHandler.Get implemented with proper parameter validation
- [ ] Response DTO created/updated for detailed container information (extends existing list DTO)
- [ ] Response returns container details with 200 status code
- [ ] Proper error handling for container not found, unauthorized access, and connection errors
- [ ] User authentication and authorization verified (user must have access to the machine)
- [ ] Container lookup uses the machine's connection context
- [ ] Unit tests for service and handler methods
- [ ] Integration test for the complete get flow

## Blocked by
- 030-container-list (container infrastructure is in place)

## Notes
This implements GET /api/machines/{id}/containers/{containerId} endpoint. Should use podman ContainerInspect or ContainerGet API to get full container details.
